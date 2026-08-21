package voice

import (
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	discardTurn "github.com/Stocist/discard/internal/turn"
	"github.com/google/uuid"
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"
)

type testVoicePeer struct {
	userID       uuid.UUID
	pc           *webrtc.PeerConnection
	mic          *webrtc.TrackLocalStaticRTP
	received     chan string
	renegotiated chan struct{}
}

func TestVoiceSessionBidirectionalDirect(t *testing.T) {
	testVoiceSessionBidirectional(t, webrtc.Configuration{})
}

func TestVoiceSessionBidirectionalTURN(t *testing.T) {
	const port = "53490"
	const secret = "voice-test-secret"
	t.Setenv("TURN_PORT", port)
	t.Setenv("TURN_SECRET", secret)
	t.Setenv("TURN_PUBLIC_IP", "127.0.0.1")
	server, serverSecret, err := discardTurn.Start("discard")
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	if serverSecret != secret {
		t.Fatalf("TURN server secret = %q, want %q", serverSecret, secret)
	}
	username, credential, _ := discardTurn.GenerateCredentials(secret)
	testVoiceSessionBidirectional(t, webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{{
			URLs:       []string{"turn:127.0.0.1:" + port + "?transport=udp"},
			Username:   username,
			Credential: credential,
		}},
		ICETransportPolicy: webrtc.ICETransportPolicyRelay,
	})
}

func testVoiceSessionBidirectional(t *testing.T, config webrtc.Configuration) {
	t.Helper()
	channelID := uuid.New()
	var peersMu sync.RWMutex
	peers := make(map[uuid.UUID]*testVoicePeer)
	var session *VoiceSession
	session = NewVoiceSession(channelID, func(userID uuid.UUID, data []byte) {
		peersMu.RLock()
		peer := peers[userID]
		peersMu.RUnlock()
		if peer == nil {
			return
		}
		go func() {
			if err := peer.handleOffer(session, data); err != nil {
				t.Errorf("renegotiation for %s: %v", userID, err)
			}
		}()
	}, func([]byte) {}, func(uuid.UUID) {})

	firstID := uuid.New()
	firstJoin, err := session.AddParticipant(firstID, "first", "")
	if err != nil {
		t.Fatal(err)
	}
	first := newTestVoicePeer(t, firstID, config)
	peersMu.Lock()
	peers[firstID] = first
	peersMu.Unlock()
	if err := first.answerInitialOffer(session, firstJoin); err != nil {
		t.Fatal(err)
	}
	waitForPeerState(t, first.pc, webrtc.PeerConnectionStateConnected)

	secondID := uuid.New()
	secondJoin, err := session.AddParticipant(secondID, "second", "")
	if err != nil {
		t.Fatal(err)
	}
	second := newTestVoicePeer(t, secondID, config)
	peersMu.Lock()
	peers[secondID] = second
	peersMu.Unlock()
	if err := second.answerInitialOffer(session, secondJoin); err != nil {
		t.Fatal(err)
	}
	waitForPeerState(t, second.pc, webrtc.PeerConnectionStateConnected)
	select {
	case <-first.renegotiated:
	case <-time.After(10 * time.Second):
		t.Fatal("first participant did not finish renegotiation")
	}

	writeTestAudio(t, first.mic)
	writeTestAudio(t, second.mic)
	waitForTrack(t, first.received, "audio-"+secondID.String())
	waitForTrack(t, second.received, "audio-"+firstID.String())

	first.pc.Close()
	second.pc.Close()
	session.RemoveParticipant(firstID)
	session.RemoveParticipant(secondID)
}

func newTestVoicePeer(t *testing.T, userID uuid.UUID, config webrtc.Configuration) *testVoicePeer {
	t.Helper()
	pc, err := webrtc.NewPeerConnection(config)
	if err != nil {
		t.Fatal(err)
	}
	peer := &testVoicePeer{
		userID: userID, pc: pc, received: make(chan string, 16), renegotiated: make(chan struct{}, 1),
	}
	pc.OnTrack(func(track *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
		go func() {
			if _, _, err := track.ReadRTP(); err == nil {
				peer.received <- track.ID()
			}
		}()
	})
	return peer
}

func (p *testVoicePeer) answerInitialOffer(session *VoiceSession, result *JoinResult) error {
	var offer webrtc.SessionDescription
	if err := json.Unmarshal(result.SDP, &offer); err != nil {
		return err
	}
	if err := p.pc.SetRemoteDescription(offer); err != nil {
		return err
	}
	mic, err := webrtc.NewTrackLocalStaticRTP(
		webrtc.RTPCodecCapability{MimeType: webrtc.MimeTypeOpus, ClockRate: 48000, Channels: 2},
		"mic-"+p.userID.String(), "voice-test",
	)
	if err != nil {
		return err
	}
	sender, err := p.pc.AddTrack(mic)
	if err != nil {
		return err
	}
	p.mic = mic
	go drainRTCP(sender)
	return p.answer(session)
}

func (p *testVoicePeer) handleOffer(session *VoiceSession, data []byte) error {
	var event struct {
		Type string                     `json:"type"`
		SDP  *webrtc.SessionDescription `json:"sdp"`
	}
	if err := json.Unmarshal(data, &event); err != nil {
		return err
	}
	if event.Type != "voice_offer" || event.SDP == nil {
		return nil
	}
	if err := p.pc.SetRemoteDescription(*event.SDP); err != nil {
		return err
	}
	if err := p.answer(session); err != nil {
		return err
	}
	select {
	case p.renegotiated <- struct{}{}:
	default:
	}
	return nil
}

func (p *testVoicePeer) answer(session *VoiceSession) error {
	answer, err := p.pc.CreateAnswer(nil)
	if err != nil {
		return err
	}
	gatherComplete := webrtc.GatheringCompletePromise(p.pc)
	if err := p.pc.SetLocalDescription(answer); err != nil {
		return err
	}
	select {
	case <-gatherComplete:
	case <-time.After(10 * time.Second):
		return fmt.Errorf("ICE gathering timed out")
	}
	raw, err := json.Marshal(p.pc.LocalDescription())
	if err != nil {
		return err
	}
	return session.HandleAnswer(p.userID, raw)
}

func drainRTCP(sender *webrtc.RTPSender) {
	buf := make([]byte, 1500)
	for {
		if _, _, err := sender.Read(buf); err != nil {
			return
		}
	}
}

func writeTestAudio(t *testing.T, track *webrtc.TrackLocalStaticRTP) {
	t.Helper()
	if track == nil {
		t.Fatal("mic track was not initialized")
	}
	for i := 0; i < 100; i++ {
		err := track.WriteRTP(&rtp.Packet{
			Header:  rtp.Header{Version: 2, PayloadType: 111, SequenceNumber: uint16(i), Timestamp: uint32(i * 960), SSRC: 1},
			Payload: []byte{0xf8, 0xff, 0xfe},
		})
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func waitForPeerState(t *testing.T, pc *webrtc.PeerConnection, want webrtc.PeerConnectionState) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if pc.ConnectionState() == want {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("peer state = %s, want %s", pc.ConnectionState(), want)
}

func waitForTrack(t *testing.T, received <-chan string, want string) {
	t.Helper()
	deadline := time.After(10 * time.Second)
	for {
		select {
		case got := <-received:
			if got == want {
				return
			}
		case <-deadline:
			t.Fatalf("did not receive track %s", want)
		}
	}
}
