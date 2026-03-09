package voice

import (
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/pion/interceptor"
	"github.com/pion/interceptor/pkg/stats"
	"github.com/pion/webrtc/v4"
)

// ParticipantState is the public view of a voice participant.
type ParticipantState struct {
	UserID     uuid.UUID `json:"user_id"`
	Username   string    `json:"username"`
	AvatarPath string    `json:"avatar_path,omitempty"`
	Muted      bool      `json:"muted"`
	Deafened   bool      `json:"deafened"`
}

// participant is the internal state for one user in a voice session.
type participant struct {
	UserID       uuid.UUID
	Username     string
	AvatarPath   string
	PC           *webrtc.PeerConnection
	Muted        bool
	Deafened     bool
	outputTracks map[uuid.UUID]*webrtc.TrackLocalStaticRTP // keyed by source user ID
}

// VoiceSession manages a single voice channel's WebRTC connections.
type VoiceSession struct {
	ChannelID    uuid.UUID
	mu           sync.RWMutex
	participants map[uuid.UUID]*participant
	sendToUser   func(userID uuid.UUID, data []byte)
	onEvent      func(data []byte)
}

// NewVoiceSession creates a voice session for the given channel.
func NewVoiceSession(channelID uuid.UUID, sendToUser func(uuid.UUID, []byte), onEvent func([]byte)) *VoiceSession {
	return &VoiceSession{
		ChannelID:    channelID,
		participants: make(map[uuid.UUID]*participant),
		sendToUser:   sendToUser,
		onEvent:      onEvent,
	}
}

func newMediaEngine() (*webrtc.MediaEngine, error) {
	m := &webrtc.MediaEngine{}
	if err := m.RegisterCodec(webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:    webrtc.MimeTypeOpus,
			ClockRate:   48000,
			Channels:    2,
			SDPFmtpLine: "minptime=10;useinbandfec=1",
		},
		PayloadType: 111,
	}, webrtc.RTPCodecTypeAudio); err != nil {
		return nil, err
	}
	return m, nil
}

func newPeerConnection(me *webrtc.MediaEngine) (*webrtc.PeerConnection, error) {
	i := &interceptor.Registry{}
	statsFactory, err := stats.NewInterceptor()
	if err != nil {
		return nil, err
	}
	i.Add(statsFactory)
	if err := webrtc.RegisterDefaultInterceptors(me, i); err != nil {
		return nil, err
	}

	api := webrtc.NewAPI(webrtc.WithMediaEngine(me), webrtc.WithInterceptorRegistry(i))
	return api.NewPeerConnection(webrtc.Configuration{})
}

// AddParticipant creates a PeerConnection for the user, sets up track forwarding,
// and returns an SDP offer to send to the client.
func (s *VoiceSession) AddParticipant(userID uuid.UUID, username, avatarPath string) (json.RawMessage, error) {
	s.mu.Lock()

	// If already present, remove first
	if existing, ok := s.participants[userID]; ok {
		s.removeParticipantLocked(existing)
	}

	me, err := newMediaEngine()
	if err != nil {
		s.mu.Unlock()
		return nil, err
	}
	pc, err := newPeerConnection(me)
	if err != nil {
		s.mu.Unlock()
		return nil, err
	}

	p := &participant{
		UserID:       userID,
		Username:     username,
		AvatarPath:   avatarPath,
		PC:           pc,
		outputTracks: make(map[uuid.UUID]*webrtc.TrackLocalStaticRTP),
	}

	// Add a recv-only transceiver so the client sends us their audio
	_, err = pc.AddTransceiverFromKind(webrtc.RTPCodecTypeAudio, webrtc.RTPTransceiverInit{
		Direction: webrtc.RTPTransceiverDirectionRecvonly,
	})
	if err != nil {
		pc.Close()
		s.mu.Unlock()
		return nil, err
	}

	// Create output tracks for existing participants → new participant hears them
	for otherID, other := range s.participants {
		track, err := webrtc.NewTrackLocalStaticRTP(
			webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus, ClockRate: 48000, Channels: 2},
			"audio-"+otherID.String(),
			"voice-"+s.ChannelID.String(),
		)
		if err != nil {
			pc.Close()
			s.mu.Unlock()
			return nil, err
		}
		if _, err := pc.AddTrack(track); err != nil {
			pc.Close()
			s.mu.Unlock()
			return nil, err
		}
		p.outputTracks[otherID] = track

		// Also create output track on other participant's PC for this new user
		otherTrack, err := webrtc.NewTrackLocalStaticRTP(
			webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus, ClockRate: 48000, Channels: 2},
			"audio-"+userID.String(),
			"voice-"+s.ChannelID.String(),
		)
		if err != nil {
			pc.Close()
			s.mu.Unlock()
			return nil, err
		}
		if _, err := other.PC.AddTrack(otherTrack); err != nil {
			pc.Close()
			s.mu.Unlock()
			return nil, err
		}
		other.outputTracks[userID] = otherTrack
	}

	// OnTrack: forward incoming RTP to all other participants' output tracks
	pc.OnTrack(func(remote *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
		log.Printf("voice: OnTrack fired for user %s, codec=%s", userID, remote.Codec().MimeType)
		go s.forwardTrack(userID, remote)
	})

	// OnICECandidate: send candidates to client
	pc.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c == nil {
			return
		}
		data, err := json.Marshal(map[string]interface{}{
			"type":       "voice_ice_candidate",
			"channel_id": s.ChannelID.String(),
			"candidate":  c.ToJSON(),
		})
		if err != nil {
			log.Printf("voice: marshal ICE candidate error: %v", err)
			return
		}
		s.sendToUser(userID, data)
	})

	// OnTrack logging
	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		log.Printf("voice: user %s PC state → %s", userID, state)
	})

	s.participants[userID] = p
	log.Printf("voice: added participant %s (%s), total=%d", userID, username, len(s.participants))

	// Collect participants that need renegotiation before releasing the lock
	othersToRenegotiate := make(map[uuid.UUID]*participant)
	for id, op := range s.participants {
		if id != userID {
			othersToRenegotiate[id] = op
		}
	}

	s.mu.Unlock()

	// Create SDP offer outside the lock — ICE gathering needs signaling to work
	offer, err := pc.CreateOffer(nil)
	if err != nil {
		s.mu.Lock()
		s.removeParticipantLocked(p)
		s.mu.Unlock()
		return nil, err
	}
	if err := pc.SetLocalDescription(offer); err != nil {
		s.mu.Lock()
		s.removeParticipantLocked(p)
		s.mu.Unlock()
		return nil, err
	}

	// Wait for ICE gathering outside the lock
	gatherComplete := webrtc.GatheringCompletePromise(pc)
	select {
	case <-gatherComplete:
	case <-time.After(5 * time.Second):
		log.Printf("voice: ICE gathering timed out for user %s", userID)
	}

	sdp, err := json.Marshal(pc.LocalDescription())
	if err != nil {
		s.mu.Lock()
		s.removeParticipantLocked(p)
		s.mu.Unlock()
		return nil, err
	}

	// Renegotiate existing participants so they get the new output track
	for id, op := range othersToRenegotiate {
		s.renegotiate(id, op)
	}

	return sdp, nil
}

// forwardTrack reads RTP packets from a remote track and writes them to all
// other participants' output tracks for this source user.
func (s *VoiceSession) forwardTrack(sourceUserID uuid.UUID, remote *webrtc.TrackRemote) {
	buf := make([]byte, 1500)
	packets := 0
	for {
		n, _, err := remote.Read(buf)
		if err != nil {
			log.Printf("voice: forwardTrack ended for user %s after %d packets: %v", sourceUserID, packets, err)
			return
		}
		packets++
		if packets == 1 || packets%500 == 0 {
			log.Printf("voice: forwarding RTP from %s, packet #%d, size=%d bytes", sourceUserID, packets, n)
		}

		s.mu.RLock()
		for otherID, other := range s.participants {
			if otherID == sourceUserID {
				continue
			}
			if other.Deafened {
				continue
			}
			if track, ok := other.outputTracks[sourceUserID]; ok {
				if _, writeErr := track.Write(buf[:n]); writeErr != nil {
					log.Printf("voice: write to output track error: %v", writeErr)
				}
			}
		}
		s.mu.RUnlock()
	}
}

// HandleAnswer sets the remote SDP answer on a participant's PeerConnection.
func (s *VoiceSession) HandleAnswer(userID uuid.UUID, sdpRaw json.RawMessage) error {
	s.mu.RLock()
	p, ok := s.participants[userID]
	s.mu.RUnlock()
	if !ok {
		return nil
	}

	var answer webrtc.SessionDescription
	if err := json.Unmarshal(sdpRaw, &answer); err != nil {
		log.Printf("voice: HandleAnswer unmarshal error for %s: %v", userID, err)
		return err
	}
	log.Printf("voice: HandleAnswer for %s, SDP type=%s, len=%d", userID, answer.Type, len(answer.SDP))
	return p.PC.SetRemoteDescription(answer)
}

// HandleICE adds an ICE candidate to a participant's PeerConnection.
func (s *VoiceSession) HandleICE(userID uuid.UUID, candidateRaw json.RawMessage) error {
	s.mu.RLock()
	p, ok := s.participants[userID]
	s.mu.RUnlock()
	if !ok {
		return nil
	}

	var candidate webrtc.ICECandidateInit
	if err := json.Unmarshal(candidateRaw, &candidate); err != nil {
		return err
	}
	return p.PC.AddICECandidate(candidate)
}

// RemoveParticipant removes a user from the voice session.
func (s *VoiceSession) RemoveParticipant(userID uuid.UUID) {
	s.mu.Lock()

	p, ok := s.participants[userID]
	if !ok {
		s.mu.Unlock()
		return
	}
	s.removeParticipantLocked(p)

	// Collect remaining participants for renegotiation
	remaining := make(map[uuid.UUID]*participant, len(s.participants))
	for id, other := range s.participants {
		remaining[id] = other
	}
	s.mu.Unlock()

	// Renegotiate remaining participants outside the lock
	for id, other := range remaining {
		s.renegotiate(id, other)
	}
}

// removeParticipantLocked closes a participant's PC and cleans up. Caller holds s.mu.
func (s *VoiceSession) removeParticipantLocked(p *participant) {
	p.PC.Close()
	// Remove output tracks and RTPSenders that other participants have for this user
	for _, other := range s.participants {
		if track, has := other.outputTracks[p.UserID]; has {
			for _, sender := range other.PC.GetSenders() {
				if sender.Track() == track {
					other.PC.RemoveTrack(sender)
					break
				}
			}
			delete(other.outputTracks, p.UserID)
		}
	}
	delete(s.participants, p.UserID)
}

// renegotiate creates a new offer and sends it to the participant.
// Must NOT be called while holding s.mu — ICE gathering needs the lock free.
func (s *VoiceSession) renegotiate(userID uuid.UUID, p *participant) {
	offer, err := p.PC.CreateOffer(nil)
	if err != nil {
		log.Printf("voice: renegotiate offer error for %s: %v", userID, err)
		return
	}
	if err := p.PC.SetLocalDescription(offer); err != nil {
		log.Printf("voice: renegotiate set local desc error for %s: %v", userID, err)
		return
	}

	// Wait for ICE gathering (outside any lock)
	gatherComplete := webrtc.GatheringCompletePromise(p.PC)
	select {
	case <-gatherComplete:
	case <-time.After(3 * time.Second):
	}

	sdp, err := json.Marshal(p.PC.LocalDescription())
	if err != nil {
		log.Printf("voice: renegotiate marshal error for %s: %v", userID, err)
		return
	}

	data, err := json.Marshal(map[string]interface{}{
		"type":       "voice_offer",
		"channel_id": s.ChannelID.String(),
		"sdp":        json.RawMessage(sdp),
	})
	if err != nil {
		return
	}
	s.sendToUser(userID, data)
}

// SetMuted sets the muted state for a participant.
func (s *VoiceSession) SetMuted(userID uuid.UUID, muted bool) {
	s.mu.Lock()
	if p, ok := s.participants[userID]; ok {
		p.Muted = muted
	}
	s.mu.Unlock()
}

// SetDeafened sets the deafened state for a participant.
func (s *VoiceSession) SetDeafened(userID uuid.UUID, deafened bool) {
	s.mu.Lock()
	if p, ok := s.participants[userID]; ok {
		p.Deafened = deafened
	}
	s.mu.Unlock()
}

// Participants returns the current participant states.
func (s *VoiceSession) Participants() []ParticipantState {
	s.mu.RLock()
	defer s.mu.RUnlock()

	states := make([]ParticipantState, 0, len(s.participants))
	for _, p := range s.participants {
		states = append(states, ParticipantState{
			UserID:     p.UserID,
			Username:   p.Username,
			AvatarPath: p.AvatarPath,
			Muted:      p.Muted,
			Deafened:   p.Deafened,
		})
	}
	return states
}

// IsEmpty returns true if the session has no participants.
func (s *VoiceSession) IsEmpty() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.participants) == 0
}
