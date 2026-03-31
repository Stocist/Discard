package voice

import (
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/pion/ice/v4"
	"github.com/pion/interceptor"
	"github.com/pion/interceptor/pkg/stats"
	"github.com/pion/webrtc/v4"
)

// ParticipantState is the public view of a voice participant.
type ParticipantState struct {
	UserID        uuid.UUID `json:"user_id"`
	Username      string    `json:"username"`
	AvatarPath    string    `json:"avatar_path,omitempty"`
	Muted         bool      `json:"muted"`
	Deafened      bool      `json:"deafened"`
	ScreenSharing bool      `json:"screen_sharing"`
	CameraOn      bool      `json:"camera_on"`
}

// participant is the internal state for one user in a voice session.
type participant struct {
	UserID        uuid.UUID
	Username      string
	AvatarPath    string
	PC            *webrtc.PeerConnection
	Muted         bool
	Deafened      bool
	ScreenSharing bool
	CameraOn      bool
	outputTracks  map[uuid.UUID]*webrtc.TrackLocalStaticRTP // keyed by source user ID
	// Screen share output tracks forwarded to this participant from the sharer
	screenVideoTracks map[uuid.UUID]*webrtc.TrackLocalStaticRTP
	screenAudioTracks map[uuid.UUID]*webrtc.TrackLocalStaticRTP
	// Camera output tracks forwarded to this participant from other users
	cameraVideoTracks map[uuid.UUID]*webrtc.TrackLocalStaticRTP
	// Recv transceivers added when this user starts screen sharing (cleaned up on stop)
	screenRecvTransceivers []*webrtc.RTPTransceiver
	// Recv transceiver added when this user starts camera (cleaned up on stop)
	cameraRecvTransceiver *webrtc.RTPTransceiver
	renego                sync.Mutex // serializes renegotiation per participant
}

// VoiceSession manages a single voice channel's WebRTC connections.
type VoiceSession struct {
	ChannelID      uuid.UUID
	mu             sync.RWMutex
	participants   map[uuid.UUID]*participant
	screenSharerID *uuid.UUID // who is currently screen sharing (nil = nobody)
	sendToUser     func(userID uuid.UUID, data []byte)
	onEvent        func(data []byte)
	onRemove       func(userID uuid.UUID) // called when a participant is removed (ICE failure, etc.)
}

// NewVoiceSession creates a voice session for the given channel.
func NewVoiceSession(channelID uuid.UUID, sendToUser func(uuid.UUID, []byte), onEvent func([]byte), onRemove func(uuid.UUID)) *VoiceSession {
	return &VoiceSession{
		ChannelID:    channelID,
		participants: make(map[uuid.UUID]*participant),
		sendToUser:   sendToUser,
		onEvent:      onEvent,
		onRemove:     onRemove,
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
	// VP8 for screen sharing video
	if err := m.RegisterCodec(webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:  webrtc.MimeTypeVP8,
			ClockRate: 90000,
		},
		PayloadType: 96,
	}, webrtc.RTPCodecTypeVideo); err != nil {
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

	se := webrtc.SettingEngine{}
	se.SetLite(true)
	se.SetICEMulticastDNSMode(ice.MulticastDNSModeDisabled)

	api := webrtc.NewAPI(
		webrtc.WithMediaEngine(me),
		webrtc.WithInterceptorRegistry(i),
		webrtc.WithSettingEngine(se),
	)
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
		UserID:            userID,
		Username:          username,
		AvatarPath:        avatarPath,
		PC:                pc,
		outputTracks:      make(map[uuid.UUID]*webrtc.TrackLocalStaticRTP),
		screenVideoTracks: make(map[uuid.UUID]*webrtc.TrackLocalStaticRTP),
		screenAudioTracks: make(map[uuid.UUID]*webrtc.TrackLocalStaticRTP),
		cameraVideoTracks: make(map[uuid.UUID]*webrtc.TrackLocalStaticRTP),
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
		// Use sendonly transceiver so client sees recvonly direction in offer
		transceiver, err := pc.AddTransceiverFromKind(webrtc.RTPCodecTypeAudio, webrtc.RTPTransceiverInit{
			Direction: webrtc.RTPTransceiverDirectionSendonly,
		})
		if err != nil {
			pc.Close()
			s.mu.Unlock()
			return nil, err
		}
		if err := transceiver.Sender().ReplaceTrack(track); err != nil {
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
		// Use sendonly transceiver so client sees recvonly direction in offer
		otherTransceiver, err := other.PC.AddTransceiverFromKind(webrtc.RTPCodecTypeAudio, webrtc.RTPTransceiverInit{
			Direction: webrtc.RTPTransceiverDirectionSendonly,
		})
		if err != nil {
			pc.Close()
			s.mu.Unlock()
			return nil, err
		}
		if err := otherTransceiver.Sender().ReplaceTrack(otherTrack); err != nil {
			pc.Close()
			s.mu.Unlock()
			return nil, err
		}
		other.outputTracks[userID] = otherTrack
	}

	// OnTrack: forward incoming RTP to all other participants' output tracks
	// Audio: first audio track = voice mic, subsequent audio = screen share system audio.
	// Video: differentiate camera vs screen by matching the receiver against the
	// camera recv transceiver stored on the participant. If the receiver's transceiver
	// matches, it's camera; otherwise it's screen share.
	voiceAudioReceived := false
	pc.OnTrack(func(remote *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		codec := remote.Codec().MimeType
		log.Printf("voice: OnTrack fired for user %s, codec=%s, streamID=%s, kind=%s", userID, codec, remote.StreamID(), remote.Kind())

		if remote.Kind() == webrtc.RTPCodecTypeVideo {
			// Check if this receiver belongs to the camera transceiver
			isCamera := false
			s.mu.RLock()
			if cp, ok := s.participants[userID]; ok && cp.cameraRecvTransceiver != nil {
				if cp.cameraRecvTransceiver.Receiver() == receiver {
					isCamera = true
				}
			}
			s.mu.RUnlock()
			if isCamera {
				go s.forwardCameraTrack(userID, remote)
			} else {
				go s.forwardScreenTrack(userID, remote)
			}
		} else if remote.Kind() == webrtc.RTPCodecTypeAudio && !voiceAudioReceived {
			// First audio track is voice
			voiceAudioReceived = true
			go s.forwardTrack(userID, remote)
		} else {
			// Subsequent audio tracks are screen share system audio
			go s.forwardScreenTrack(userID, remote)
		}
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

	// Connection state: evict on failed/closed, grace period on disconnected
	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		log.Printf("voice: user %s PC state → %s", userID, state)
		switch state {
		case webrtc.PeerConnectionStateFailed, webrtc.PeerConnectionStateClosed:
			s.RemoveParticipant(userID)
			if s.onRemove != nil {
				s.onRemove(userID)
			}
		case webrtc.PeerConnectionStateDisconnected:
			capturedP := p // capture pointer to detect rejoin
			go func() {
				time.Sleep(5 * time.Second)
				s.mu.RLock()
				currentP, ok := s.participants[userID]
				s.mu.RUnlock()
				// Only evict if same participant (not a rejoin) and still not connected
				if ok && currentP == capturedP && currentP.PC.ConnectionState() != webrtc.PeerConnectionStateConnected {
					log.Printf("voice: user %s still disconnected after grace period, removing", userID)
					s.RemoveParticipant(userID)
					if s.onRemove != nil {
						s.onRemove(userID)
					}
				}
			}()
		}
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

	// cleanupOnFailure removes the partially-added participant and renegotiates others
	// to remove the output tracks that were added to their PCs.
	cleanupOnFailure := func() {
		s.mu.Lock()
		s.removeParticipantLocked(p)
		remaining := make(map[uuid.UUID]*participant, len(s.participants))
		for id, op := range s.participants {
			remaining[id] = op
		}
		s.mu.Unlock()
		for id, op := range remaining {
			s.renegotiate(id, op)
		}
	}

	// Create SDP offer outside the lock — ICE gathering needs signaling to work
	offer, err := pc.CreateOffer(nil)
	if err != nil {
		cleanupOnFailure()
		return nil, err
	}
	if err := pc.SetLocalDescription(offer); err != nil {
		cleanupOnFailure()
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
		cleanupOnFailure()
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

	// Grab a reference to the participant's PC so we can check its state
	s.mu.RLock()
	p, ok := s.participants[sourceUserID]
	s.mu.RUnlock()
	if !ok {
		return
	}
	srcPC := p.PC

	for {
		// Exit if the PeerConnection is no longer connected
		state := srcPC.ConnectionState()
		if state == webrtc.PeerConnectionStateFailed ||
			state == webrtc.PeerConnectionStateClosed {
			log.Printf("voice: forwardTrack exiting for user %s, PC state=%s, packets=%d", sourceUserID, state, packets)
			return
		}

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
	// If this participant was the screen sharer, clear it
	if s.screenSharerID != nil && *s.screenSharerID == p.UserID {
		s.screenSharerID = nil
	}
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
		// Remove screen share tracks
		s.removeScreenTracksFromParticipant(other, p.UserID)
		// Remove camera tracks
		s.removeCameraTracksFromParticipant(other, p.UserID)
	}
	delete(s.participants, p.UserID)
}

// removeScreenTracksFromParticipant removes screen share video/audio tracks
// for a given sharer from a viewer's PC. Caller holds s.mu.
func (s *VoiceSession) removeScreenTracksFromParticipant(viewer *participant, sharerID uuid.UUID) {
	if track, has := viewer.screenVideoTracks[sharerID]; has {
		for _, sender := range viewer.PC.GetSenders() {
			if sender.Track() == track {
				viewer.PC.RemoveTrack(sender)
				break
			}
		}
		delete(viewer.screenVideoTracks, sharerID)
	}
	if track, has := viewer.screenAudioTracks[sharerID]; has {
		for _, sender := range viewer.PC.GetSenders() {
			if sender.Track() == track {
				viewer.PC.RemoveTrack(sender)
				break
			}
		}
		delete(viewer.screenAudioTracks, sharerID)
	}
}

// renegotiate creates a new offer and sends it to the participant.
// Must NOT be called while holding s.mu — ICE gathering needs the lock free.
// Uses per-participant mutex to prevent concurrent renegotiation race conditions.
func (s *VoiceSession) renegotiate(userID uuid.UUID, p *participant) {
	p.renego.Lock()
	defer p.renego.Unlock()

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
			UserID:        p.UserID,
			Username:      p.Username,
			AvatarPath:    p.AvatarPath,
			Muted:         p.Muted,
			Deafened:      p.Deafened,
			ScreenSharing: p.ScreenSharing,
			CameraOn:      p.CameraOn,
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

// ScreenSharerID returns the user ID of the current screen sharer, or nil.
func (s *VoiceSession) ScreenSharerID() *uuid.UUID {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.screenSharerID
}

// StartScreenShare marks a user as screen sharing and sets up receive transceivers
// for the screen video+audio, plus output tracks to all other participants.
// Returns true if successful, false if someone else is already sharing.
func (s *VoiceSession) StartScreenShare(userID uuid.UUID) bool {
	s.mu.Lock()

	// Only one screen share at a time
	if s.screenSharerID != nil && *s.screenSharerID != userID {
		s.mu.Unlock()
		return false
	}

	p, ok := s.participants[userID]
	if !ok {
		s.mu.Unlock()
		return false
	}

	id := userID
	s.screenSharerID = &id
	p.ScreenSharing = true

	// Add recv-only transceivers for screen video and screen audio
	videoTr, err := p.PC.AddTransceiverFromKind(webrtc.RTPCodecTypeVideo, webrtc.RTPTransceiverInit{
		Direction: webrtc.RTPTransceiverDirectionRecvonly,
	})
	if err != nil {
		log.Printf("voice: screen share add video transceiver error: %v", err)
		s.screenSharerID = nil
		p.ScreenSharing = false
		s.mu.Unlock()
		return false
	}
	audioTr, err := p.PC.AddTransceiverFromKind(webrtc.RTPCodecTypeAudio, webrtc.RTPTransceiverInit{
		Direction: webrtc.RTPTransceiverDirectionRecvonly,
	})
	if err != nil {
		log.Printf("voice: screen share add audio transceiver error: %v", err)
		s.screenSharerID = nil
		p.ScreenSharing = false
		s.mu.Unlock()
		return false
	}
	p.screenRecvTransceivers = []*webrtc.RTPTransceiver{videoTr, audioTr}

	// Create output tracks on all other participants for screen video+audio
	for otherID, other := range s.participants {
		if otherID == userID {
			continue
		}
		s.addScreenOutputTracks(other, userID)
	}

	// Collect all participants for renegotiation
	allParticipants := make(map[uuid.UUID]*participant, len(s.participants))
	for id, p := range s.participants {
		allParticipants[id] = p
	}
	s.mu.Unlock()

	// Renegotiate all participants (sharer needs recv transceivers, viewers need send)
	for id, p := range allParticipants {
		s.renegotiate(id, p)
	}

	return true
}

// addScreenOutputTracks creates sendonly video+audio tracks on viewer's PC for screen share.
// Caller must hold s.mu.
func (s *VoiceSession) addScreenOutputTracks(viewer *participant, sharerID uuid.UUID) {
	// Video track
	videoTrack, err := webrtc.NewTrackLocalStaticRTP(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeVP8, ClockRate: 90000},
		"screen-video-"+sharerID.String(),
		"screen-"+s.ChannelID.String(),
	)
	if err != nil {
		log.Printf("voice: create screen video track error: %v", err)
		return
	}
	vt, err := viewer.PC.AddTransceiverFromKind(webrtc.RTPCodecTypeVideo, webrtc.RTPTransceiverInit{
		Direction: webrtc.RTPTransceiverDirectionSendonly,
	})
	if err != nil {
		log.Printf("voice: add screen video transceiver error: %v", err)
		return
	}
	if err := vt.Sender().ReplaceTrack(videoTrack); err != nil {
		log.Printf("voice: replace screen video track error: %v", err)
		return
	}
	viewer.screenVideoTracks[sharerID] = videoTrack

	// Audio track (system audio from screen share)
	audioTrack, err := webrtc.NewTrackLocalStaticRTP(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus, ClockRate: 48000, Channels: 2},
		"screen-audio-"+sharerID.String(),
		"screen-"+s.ChannelID.String(),
	)
	if err != nil {
		log.Printf("voice: create screen audio track error: %v", err)
		return
	}
	at, err := viewer.PC.AddTransceiverFromKind(webrtc.RTPCodecTypeAudio, webrtc.RTPTransceiverInit{
		Direction: webrtc.RTPTransceiverDirectionSendonly,
	})
	if err != nil {
		log.Printf("voice: add screen audio transceiver error: %v", err)
		return
	}
	if err := at.Sender().ReplaceTrack(audioTrack); err != nil {
		log.Printf("voice: replace screen audio track error: %v", err)
		return
	}
	viewer.screenAudioTracks[sharerID] = audioTrack
}

// StopScreenShare stops screen sharing for a user.
func (s *VoiceSession) StopScreenShare(userID uuid.UUID) {
	s.mu.Lock()

	if s.screenSharerID == nil || *s.screenSharerID != userID {
		s.mu.Unlock()
		return
	}

	p, ok := s.participants[userID]
	if ok {
		p.ScreenSharing = false
		// Stop and clean up recv transceivers on the sharer's PC
		for _, tr := range p.screenRecvTransceivers {
			if err := tr.Stop(); err != nil {
				log.Printf("voice: stop screen transceiver error: %v", err)
			}
		}
		p.screenRecvTransceivers = nil
	}
	s.screenSharerID = nil

	// Remove screen share output tracks from all viewers
	for otherID, other := range s.participants {
		if otherID == userID {
			continue
		}
		s.removeScreenTracksFromParticipant(other, userID)
	}

	// Collect all participants for renegotiation (including sharer)
	allParticipants := make(map[uuid.UUID]*participant, len(s.participants))
	for id, p := range s.participants {
		allParticipants[id] = p
	}
	s.mu.Unlock()

	// Renegotiate all participants (sharer to remove recv, viewers to remove send)
	for id, p := range allParticipants {
		s.renegotiate(id, p)
	}
}

// StartCamera marks a user as camera-on, adds a recv-only video transceiver on
// their PC, and creates output video tracks for all other participants.
func (s *VoiceSession) StartCamera(userID uuid.UUID) {
	s.mu.Lock()

	p, ok := s.participants[userID]
	if !ok {
		s.mu.Unlock()
		return
	}

	p.CameraOn = true

	// Add recv-only video transceiver for camera
	videoTr, err := p.PC.AddTransceiverFromKind(webrtc.RTPCodecTypeVideo, webrtc.RTPTransceiverInit{
		Direction: webrtc.RTPTransceiverDirectionRecvonly,
	})
	if err != nil {
		log.Printf("voice: camera add video transceiver error: %v", err)
		p.CameraOn = false
		s.mu.Unlock()
		return
	}
	p.cameraRecvTransceiver = videoTr

	// Create output video tracks on all other participants
	for otherID, other := range s.participants {
		if otherID == userID {
			continue
		}
		s.addCameraOutputTrack(other, userID)
	}

	// Collect all participants for renegotiation
	allParticipants := make(map[uuid.UUID]*participant, len(s.participants))
	for id, p := range s.participants {
		allParticipants[id] = p
	}
	s.mu.Unlock()

	// Renegotiate all (sender needs recv transceiver, viewers need send)
	for id, p := range allParticipants {
		s.renegotiate(id, p)
	}
}

// addCameraOutputTrack creates a sendonly video track on viewer's PC for camera.
// Caller must hold s.mu.
func (s *VoiceSession) addCameraOutputTrack(viewer *participant, cameraUserID uuid.UUID) {
	videoTrack, err := webrtc.NewTrackLocalStaticRTP(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeVP8, ClockRate: 90000},
		"camera-video-"+cameraUserID.String(),
		"camera-"+cameraUserID.String(),
	)
	if err != nil {
		log.Printf("voice: create camera video track error: %v", err)
		return
	}
	vt, err := viewer.PC.AddTransceiverFromKind(webrtc.RTPCodecTypeVideo, webrtc.RTPTransceiverInit{
		Direction: webrtc.RTPTransceiverDirectionSendonly,
	})
	if err != nil {
		log.Printf("voice: add camera video transceiver error: %v", err)
		return
	}
	if err := vt.Sender().ReplaceTrack(videoTrack); err != nil {
		log.Printf("voice: replace camera video track error: %v", err)
		return
	}
	viewer.cameraVideoTracks[cameraUserID] = videoTrack
}

// StopCamera stops camera for a user.
func (s *VoiceSession) StopCamera(userID uuid.UUID) {
	s.mu.Lock()

	p, ok := s.participants[userID]
	if !ok || !p.CameraOn {
		s.mu.Unlock()
		return
	}

	p.CameraOn = false
	// Stop and clean up recv transceiver on the camera user's PC
	if p.cameraRecvTransceiver != nil {
		if err := p.cameraRecvTransceiver.Stop(); err != nil {
			log.Printf("voice: stop camera transceiver error: %v", err)
		}
		p.cameraRecvTransceiver = nil
	}

	// Remove camera output tracks from all viewers
	for otherID, other := range s.participants {
		if otherID == userID {
			continue
		}
		s.removeCameraTracksFromParticipant(other, userID)
	}

	// Collect all participants for renegotiation
	allParticipants := make(map[uuid.UUID]*participant, len(s.participants))
	for id, p := range s.participants {
		allParticipants[id] = p
	}
	s.mu.Unlock()

	for id, p := range allParticipants {
		s.renegotiate(id, p)
	}
}

// removeCameraTracksFromParticipant removes camera video tracks for a given
// camera user from a viewer's PC. Caller holds s.mu.
func (s *VoiceSession) removeCameraTracksFromParticipant(viewer *participant, cameraUserID uuid.UUID) {
	if track, has := viewer.cameraVideoTracks[cameraUserID]; has {
		for _, sender := range viewer.PC.GetSenders() {
			if sender.Track() == track {
				viewer.PC.RemoveTrack(sender)
				break
			}
		}
		delete(viewer.cameraVideoTracks, cameraUserID)
	}
}

// forwardCameraTrack reads RTP from a camera video remote track and writes
// to all other participants' camera output tracks.
func (s *VoiceSession) forwardCameraTrack(sourceUserID uuid.UUID, remote *webrtc.TrackRemote) {
	buf := make([]byte, 4096) // VP8 keyframes can exceed standard MTU
	packets := 0

	s.mu.RLock()
	p, ok := s.participants[sourceUserID]
	s.mu.RUnlock()
	if !ok {
		return
	}
	srcPC := p.PC

	for {
		state := srcPC.ConnectionState()
		if state == webrtc.PeerConnectionStateFailed ||
			state == webrtc.PeerConnectionStateClosed {
			log.Printf("voice: camera forwardTrack exiting for user %s, PC state=%s, packets=%d", sourceUserID, state, packets)
			return
		}

		n, _, err := remote.Read(buf)
		if err != nil {
			log.Printf("voice: camera forwardTrack ended for user %s after %d packets: %v", sourceUserID, packets, err)
			return
		}
		packets++
		if packets == 1 || packets%500 == 0 {
			log.Printf("voice: forwarding camera RTP from %s, packet #%d, size=%d bytes", sourceUserID, packets, n)
		}

		s.mu.RLock()
		for otherID, other := range s.participants {
			if otherID == sourceUserID {
				continue
			}
			if track := other.cameraVideoTracks[sourceUserID]; track != nil {
				if _, writeErr := track.Write(buf[:n]); writeErr != nil {
					log.Printf("voice: write camera track error: %v", writeErr)
				}
			}
		}
		s.mu.RUnlock()
	}
}

// forwardScreenTrack reads RTP from a screen share remote track and writes
// to all other participants' screen output tracks.
func (s *VoiceSession) forwardScreenTrack(sourceUserID uuid.UUID, remote *webrtc.TrackRemote) {
	isVideo := remote.Kind() == webrtc.RTPCodecTypeVideo
	label := "screen-audio"
	if isVideo {
		label = "screen-video"
	}

	bufSize := 1500
	if isVideo {
		bufSize = 4096 // VP8 keyframes can exceed standard MTU
	}
	buf := make([]byte, bufSize)
	packets := 0

	s.mu.RLock()
	p, ok := s.participants[sourceUserID]
	s.mu.RUnlock()
	if !ok {
		return
	}
	srcPC := p.PC

	for {
		state := srcPC.ConnectionState()
		if state == webrtc.PeerConnectionStateFailed ||
			state == webrtc.PeerConnectionStateClosed {
			log.Printf("voice: %s forwardTrack exiting for user %s, PC state=%s, packets=%d", label, sourceUserID, state, packets)
			return
		}

		n, _, err := remote.Read(buf)
		if err != nil {
			log.Printf("voice: %s forwardTrack ended for user %s after %d packets: %v", label, sourceUserID, packets, err)
			return
		}
		packets++
		if packets == 1 || packets%500 == 0 {
			log.Printf("voice: forwarding %s RTP from %s, packet #%d, size=%d bytes", label, sourceUserID, packets, n)
		}

		s.mu.RLock()
		for otherID, other := range s.participants {
			if otherID == sourceUserID {
				continue
			}
			var track *webrtc.TrackLocalStaticRTP
			if isVideo {
				track = other.screenVideoTracks[sourceUserID]
			} else {
				track = other.screenAudioTracks[sourceUserID]
			}
			if track != nil {
				if _, writeErr := track.Write(buf[:n]); writeErr != nil {
					log.Printf("voice: write %s track error: %v", label, writeErr)
				}
			}
		}
		s.mu.RUnlock()
	}
}
