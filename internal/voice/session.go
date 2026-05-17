package voice

import (
	"encoding/json"
	"log"
	"net"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/pion/ice/v4"
	"github.com/pion/interceptor"
	"github.com/pion/interceptor/pkg/stats"
	"github.com/pion/rtcp"
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

// JoinResult is returned from AddParticipant with the SDP offer and transceiver MIDs.
type JoinResult struct {
	SDP             json.RawMessage
	TransceiverMIDs map[string]string // mic_audio, camera_video, screen_video, screen_audio → MID strings
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
	// Pre-allocated recv transceivers (created at join, never removed)
	audioRecvTransceiver       *webrtc.RTPTransceiver
	cameraRecvTransceiver      *webrtc.RTPTransceiver
	screenVideoRecvTransceiver *webrtc.RTPTransceiver
	screenAudioRecvTransceiver *webrtc.RTPTransceiver
	renego                     sync.Mutex // serializes renegotiation per participant
	needsRenegotiation         bool       // set when renegotiation was deferred
	pendingNewUser             uuid.UUID  // user whose tracks need adding on deferred renego
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

// gatherHostIPs returns non-loopback IPv4 addresses from all interfaces,
// including POINTOPOINT (tailscale0) which Pion normally skips.
func gatherHostIPs() []string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	var ips []string
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}
			ip := ipNet.IP.To4()
			if ip == nil {
				continue
			}
			ips = append(ips, ip.String())
		}
	}
	return ips
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
	se.SetICEMulticastDNSMode(ice.MulticastDNSModeDisabled)

	// Include all interface IPs (including POINTOPOINT like tailscale0)
	// so browsers can reach the server via Tailscale.
	if ips := gatherHostIPs(); len(ips) > 0 {
		se.SetNAT1To1IPs(ips, webrtc.ICECandidateTypeHost)
	}

	api := webrtc.NewAPI(
		webrtc.WithMediaEngine(me),
		webrtc.WithInterceptorRegistry(i),
		webrtc.WithSettingEngine(se),
	)
	return api.NewPeerConnection(webrtc.Configuration{})
}

// AddParticipant creates a PeerConnection for the user, pre-allocates all 4 recv
// transceivers (mic, camera, screen video, screen audio), sets up track forwarding,
// and returns a JoinResult with the SDP offer and transceiver MIDs.
func (s *VoiceSession) AddParticipant(userID uuid.UUID, username, avatarPath string) (*JoinResult, error) {
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

	// Pre-allocate all 4 recv-only transceivers so the client can send media
	// without renegotiation. Order: mic audio, camera video, screen video, screen audio.
	audioRecvTr, err := pc.AddTransceiverFromKind(webrtc.RTPCodecTypeAudio, webrtc.RTPTransceiverInit{
		Direction: webrtc.RTPTransceiverDirectionRecvonly,
	})
	if err != nil {
		pc.Close()
		s.mu.Unlock()
		return nil, err
	}
	p.audioRecvTransceiver = audioRecvTr

	cameraRecvTr, err := pc.AddTransceiverFromKind(webrtc.RTPCodecTypeVideo, webrtc.RTPTransceiverInit{
		Direction: webrtc.RTPTransceiverDirectionRecvonly,
	})
	if err != nil {
		pc.Close()
		s.mu.Unlock()
		return nil, err
	}
	p.cameraRecvTransceiver = cameraRecvTr

	screenVideoRecvTr, err := pc.AddTransceiverFromKind(webrtc.RTPCodecTypeVideo, webrtc.RTPTransceiverInit{
		Direction: webrtc.RTPTransceiverDirectionRecvonly,
	})
	if err != nil {
		pc.Close()
		s.mu.Unlock()
		return nil, err
	}
	p.screenVideoRecvTransceiver = screenVideoRecvTr

	screenAudioRecvTr, err := pc.AddTransceiverFromKind(webrtc.RTPCodecTypeAudio, webrtc.RTPTransceiverInit{
		Direction: webrtc.RTPTransceiverDirectionRecvonly,
	})
	if err != nil {
		pc.Close()
		s.mu.Unlock()
		return nil, err
	}
	p.screenAudioRecvTransceiver = screenAudioRecvTr

	// Create output tracks on the NEW participant's PC for each existing participant.
	// We do NOT touch existing participants' PCs here — their output tracks for
	// the new user are added during renegotiation (addOutputTracksForUser) to avoid
	// corrupting their signaling state.
	for otherID, _ := range s.participants {
		if err := s.addOutputTracksForUser(p, otherID); err != nil {
			pc.Close()
			s.mu.Unlock()
			return nil, err
		}
	}

	// OnTrack: match incoming tracks to pre-allocated recv transceivers
	pc.OnTrack(func(remote *webrtc.TrackRemote, receiver *webrtc.RTPReceiver) {
		codec := remote.Codec().MimeType
		log.Printf("voice: OnTrack fired for user %s, codec=%s, streamID=%s, kind=%s", userID, codec, remote.StreamID(), remote.Kind())

		s.mu.RLock()
		cp, ok := s.participants[userID]
		s.mu.RUnlock()
		if !ok {
			return
		}

		switch {
		case cp.cameraRecvTransceiver != nil && cp.cameraRecvTransceiver.Receiver() == receiver:
			go s.forwardCameraTrack(userID, remote)
		case cp.screenVideoRecvTransceiver != nil && cp.screenVideoRecvTransceiver.Receiver() == receiver:
			go s.forwardScreenTrack(userID, remote)
		case cp.screenAudioRecvTransceiver != nil && cp.screenAudioRecvTransceiver.Receiver() == receiver:
			go s.forwardScreenTrack(userID, remote)
		default:
			// Default = mic audio (the first recv transceiver)
			go s.forwardTrack(userID, remote)
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

	// Connection state: only evict on failed/closed.
	// Disconnected is transient (ICE consent checks, brief network blips) and
	// recovers on its own — the sweeper catches truly dead connections.
	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		log.Printf("voice: user %s PC state → %s (trigger: OnConnectionStateChange)", userID, state)
		switch state {
		case webrtc.PeerConnectionStateConnected:
			// If renegotiation was deferred waiting for ICE to connect, run it now
			if p.needsRenegotiation {
				p.needsRenegotiation = false
				newUser := p.pendingNewUser
				p.pendingNewUser = uuid.Nil
				log.Printf("voice: user %s now connected, running deferred renegotiate (newUser=%s)", userID, newUser)
				go s.renegotiate(userID, p, newUser)
			}
		case webrtc.PeerConnectionStateFailed:
			log.Printf("voice: user %s PC FAILED, removing participant", userID)
			s.RemoveParticipant(userID)
			if s.onRemove != nil {
				s.onRemove(userID)
			}
		case webrtc.PeerConnectionStateClosed:
			log.Printf("voice: user %s PC CLOSED (already cleaned up)", userID)
			if s.onRemove != nil {
				s.onRemove(userID)
			}
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
			s.renegotiate(id, op, uuid.Nil)
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

	// Extract MIDs from the pre-allocated recv transceivers
	mids := map[string]string{
		"mic_audio":    audioRecvTr.Mid(),
		"camera_video": cameraRecvTr.Mid(),
		"screen_video": screenVideoRecvTr.Mid(),
		"screen_audio": screenAudioRecvTr.Mid(),
	}

	// Renegotiate existing participants so they get the new output tracks.
	// This is where output tracks for the new user get added to existing PCs
	// (inside renegotiate, once their signaling state is stable).
	//
	// MUST run off the caller's goroutine: AddParticipant is called from the
	// joining user's WebSocket ReadPump, which is strictly sequential per
	// connection. renegotiate blocks on ICE gathering, so doing it inline
	// stalls the joiner's ReadPump AND delays the joiner's own offer (sent
	// only after Join returns) — which deadlocks the whole signaling exchange.
	// Each renegotiate is independently serialized by p.renego, so a detached
	// goroutine here is safe.
	go func() {
		for id, op := range othersToRenegotiate {
			s.renegotiate(id, op, userID)
		}
	}()

	return &JoinResult{SDP: sdp, TransceiverMIDs: mids}, nil
}

// addOutputTracksForUser creates 4 sendonly output tracks (audio, camera, screen video,
// screen audio) on viewer's PC for sourceID. Caller must hold s.mu.
func (s *VoiceSession) addOutputTracksForUser(viewer *participant, sourceID uuid.UUID) error {
	pc := viewer.PC

	// Audio
	audioTrack, err := webrtc.NewTrackLocalStaticRTP(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus, ClockRate: 48000, Channels: 2},
		"audio-"+sourceID.String(),
		"voice-"+s.ChannelID.String(),
	)
	if err != nil {
		return err
	}
	tr, err := pc.AddTransceiverFromKind(webrtc.RTPCodecTypeAudio, webrtc.RTPTransceiverInit{
		Direction: webrtc.RTPTransceiverDirectionSendonly,
	})
	if err != nil {
		return err
	}
	if err := tr.Sender().ReplaceTrack(audioTrack); err != nil {
		return err
	}
	viewer.outputTracks[sourceID] = audioTrack

	// Camera video
	camTrack, err := webrtc.NewTrackLocalStaticRTP(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeVP8, ClockRate: 90000},
		"camera-video-"+sourceID.String(),
		"camera-"+sourceID.String(),
	)
	if err != nil {
		return err
	}
	camTr, err := pc.AddTransceiverFromKind(webrtc.RTPCodecTypeVideo, webrtc.RTPTransceiverInit{
		Direction: webrtc.RTPTransceiverDirectionSendonly,
	})
	if err != nil {
		return err
	}
	if err := camTr.Sender().ReplaceTrack(camTrack); err != nil {
		return err
	}
	viewer.cameraVideoTracks[sourceID] = camTrack

	// Screen video
	svTrack, err := webrtc.NewTrackLocalStaticRTP(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeVP8, ClockRate: 90000},
		"screen-video-"+sourceID.String(),
		"screen-"+sourceID.String(),
	)
	if err != nil {
		return err
	}
	svTr, err := pc.AddTransceiverFromKind(webrtc.RTPCodecTypeVideo, webrtc.RTPTransceiverInit{
		Direction: webrtc.RTPTransceiverDirectionSendonly,
	})
	if err != nil {
		return err
	}
	if err := svTr.Sender().ReplaceTrack(svTrack); err != nil {
		return err
	}
	viewer.screenVideoTracks[sourceID] = svTrack

	// Screen audio
	saTrack, err := webrtc.NewTrackLocalStaticRTP(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus, ClockRate: 48000, Channels: 2},
		"screen-audio-"+sourceID.String(),
		"screen-"+sourceID.String(),
	)
	if err != nil {
		return err
	}
	saTr, err := pc.AddTransceiverFromKind(webrtc.RTPCodecTypeAudio, webrtc.RTPTransceiverInit{
		Direction: webrtc.RTPTransceiverDirectionSendonly,
	})
	if err != nil {
		return err
	}
	if err := saTr.Sender().ReplaceTrack(saTrack); err != nil {
		return err
	}
	viewer.screenAudioTracks[sourceID] = saTrack

	return nil
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
// If a renegotiation was deferred while this answer was pending, it retries.
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
	if err := p.PC.SetRemoteDescription(answer); err != nil {
		return err
	}

	// If renegotiation was deferred, retry now that signaling is stable.
	if p.needsRenegotiation {
		p.needsRenegotiation = false
		newUser := p.pendingNewUser
		p.pendingNewUser = uuid.Nil
		go s.renegotiate(userID, p, newUser)
	}
	return nil
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
		log.Printf("voice: RemoveParticipant called for %s but not found in session", userID)
		return
	}
	log.Printf("voice: RemoveParticipant for %s (%s)", userID, p.Username)
	s.removeParticipantLocked(p)

	// Collect remaining participants for renegotiation
	remaining := make(map[uuid.UUID]*participant, len(s.participants))
	for id, other := range s.participants {
		remaining[id] = other
	}
	s.mu.Unlock()

	// Renegotiate remaining participants outside the lock
	for id, other := range remaining {
		s.renegotiate(id, other, uuid.Nil)
	}
}

// removeParticipantLocked closes a participant's PC and cleans up. Caller holds s.mu.
func (s *VoiceSession) removeParticipantLocked(p *participant) {
	// If this participant was the screen sharer, clear it
	if s.screenSharerID != nil && *s.screenSharerID == p.UserID {
		s.screenSharerID = nil
	}
	log.Printf("voice: removeParticipantLocked closing PC for %s (%s), PC state=%s", p.UserID, p.Username, p.PC.ConnectionState())
	p.PC.Close()
	// Remove all output tracks that other participants have for this user
	for _, other := range s.participants {
		removeAndCleanTrack(other, other.outputTracks, p.UserID)
		removeAndCleanTrack(other, other.cameraVideoTracks, p.UserID)
		removeAndCleanTrack(other, other.screenVideoTracks, p.UserID)
		removeAndCleanTrack(other, other.screenAudioTracks, p.UserID)
	}
	delete(s.participants, p.UserID)
}

// removeAndCleanTrack removes a track for sourceID from the viewer's track map
// and removes the corresponding RTP sender from the viewer's PeerConnection.
func removeAndCleanTrack(viewer *participant, tracks map[uuid.UUID]*webrtc.TrackLocalStaticRTP, sourceID uuid.UUID) {
	if track, has := tracks[sourceID]; has {
		for _, sender := range viewer.PC.GetSenders() {
			if sender.Track() == track {
				viewer.PC.RemoveTrack(sender)
				break
			}
		}
		delete(tracks, sourceID)
	}
}

// renegotiate creates a new offer and sends it to the participant.
// newUserID is the user who just joined (output tracks for them will be added
// to this participant's PC before creating the offer). Use uuid.Nil if this
// renegotiation is for a removal (no tracks to add).
// Must NOT be called while holding s.mu — ICE gathering needs the lock free.
// Uses per-participant mutex to prevent concurrent renegotiation race conditions.
func (s *VoiceSession) renegotiate(userID uuid.UUID, p *participant, newUserID uuid.UUID) {
	p.renego.Lock()
	defer p.renego.Unlock()

	// Defer if signaling isn't stable (pending offer/answer exchange)
	if p.PC.SignalingState() != webrtc.SignalingStateStable {
		p.needsRenegotiation = true
		p.pendingNewUser = newUserID
		log.Printf("voice: deferring renegotiate for %s, signaling state=%s", userID, p.PC.SignalingState())
		return
	}

	// Defer if ICE hasn't finished connecting yet — a new offer would restart ICE
	// and kill the in-progress connection attempt.
	pcState := p.PC.ConnectionState()
	if pcState != webrtc.PeerConnectionStateConnected {
		p.needsRenegotiation = true
		p.pendingNewUser = newUserID
		log.Printf("voice: deferring renegotiate for %s, PC state=%s (waiting for connected)", userID, pcState)
		return
	}

	log.Printf("voice: renegotiate START for %s, PC state=%s, signaling=%s, newUser=%s", userID, p.PC.ConnectionState(), p.PC.SignalingState(), newUserID)

	// Add output tracks for the new user on this participant's PC
	// (we couldn't do this during AddParticipant because the PC wasn't in stable state)
	if newUserID != uuid.Nil {
		if _, already := p.outputTracks[newUserID]; !already {
			s.mu.Lock()
			err := s.addOutputTracksForUser(p, newUserID)
			s.mu.Unlock()
			if err != nil {
				log.Printf("voice: renegotiate add tracks error for %s: %v", userID, err)
				return
			}
			log.Printf("voice: renegotiate added output tracks for %s on %s's PC", newUserID, userID)
		}
	}

	log.Printf("voice: renegotiate creating offer for %s, PC state=%s, senders=%d, receivers=%d",
		userID, p.PC.ConnectionState(), len(p.PC.GetSenders()), len(p.PC.GetReceivers()))

	// Log current vs new transceiver count
	transceivers := p.PC.GetTransceivers()
	log.Printf("voice: renegotiate %s has %d transceivers before offer", userID, len(transceivers))
	for i, t := range transceivers {
		log.Printf("voice:   transceiver[%d] mid=%s dir=%s kind=%s", i, t.Mid(), t.Direction(), t.Kind())
	}

	offer, err := p.PC.CreateOffer(nil)
	if err != nil {
		log.Printf("voice: renegotiate CreateOffer error for %s: %v", userID, err)
		return
	}
	log.Printf("voice: renegotiate CreateOffer OK for %s, SDP len=%d, PC state=%s", userID, len(offer.SDP), p.PC.ConnectionState())

	if err := p.PC.SetLocalDescription(offer); err != nil {
		log.Printf("voice: renegotiate SetLocalDescription error for %s: %v", userID, err)
		return
	}
	log.Printf("voice: renegotiate SetLocalDescription OK for %s, PC state=%s", userID, p.PC.ConnectionState())

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

// StartScreenShare marks a user as screen sharing. Transceivers are pre-allocated,
// so no renegotiation is needed — the client just calls replaceTrack.
// Returns true if successful, false if someone else is already sharing.
func (s *VoiceSession) StartScreenShare(userID uuid.UUID) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.screenSharerID != nil && *s.screenSharerID != userID {
		return false
	}
	p, ok := s.participants[userID]
	if !ok {
		return false
	}
	id := userID
	s.screenSharerID = &id
	p.ScreenSharing = true
	return true
}

// StopScreenShare stops screen sharing for a user. No renegotiation needed.
func (s *VoiceSession) StopScreenShare(userID uuid.UUID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.screenSharerID == nil || *s.screenSharerID != userID {
		return
	}
	p, ok := s.participants[userID]
	if ok {
		p.ScreenSharing = false
	}
	s.screenSharerID = nil
}

// StartCamera marks a user as camera-on. Transceivers are pre-allocated,
// so no renegotiation is needed — the client just calls replaceTrack.
func (s *VoiceSession) StartCamera(userID uuid.UUID) {
	s.mu.Lock()
	p, ok := s.participants[userID]
	if !ok {
		s.mu.Unlock()
		return
	}
	p.CameraOn = true
	// Send PLI for keyframe when camera resumes
	if p.cameraRecvTransceiver != nil {
		if recv := p.cameraRecvTransceiver.Receiver(); recv != nil {
			if track := recv.Track(); track != nil {
				requestKeyframe(p.PC, track)
			}
		}
	}
	s.mu.Unlock()
}

// StopCamera stops camera for a user. No renegotiation needed.
func (s *VoiceSession) StopCamera(userID uuid.UUID) {
	s.mu.Lock()
	p, ok := s.participants[userID]
	if !ok {
		s.mu.Unlock()
		return
	}
	p.CameraOn = false
	s.mu.Unlock()
}

// requestKeyframe sends an RTCP PLI to the sender so the encoder generates a keyframe.
func requestKeyframe(pc *webrtc.PeerConnection, remote *webrtc.TrackRemote) {
	if err := pc.WriteRTCP([]rtcp.Packet{
		&rtcp.PictureLossIndication{MediaSSRC: uint32(remote.SSRC())},
	}); err != nil {
		log.Printf("voice: PLI send error: %v", err)
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

	// Request an immediate keyframe so viewers can start decoding
	requestKeyframe(srcPC, remote)

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

	// Request an immediate keyframe for video so viewers can start decoding
	if isVideo {
		requestKeyframe(srcPC, remote)
	}

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
