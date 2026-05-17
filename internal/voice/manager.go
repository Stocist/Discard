package voice

import (
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/pion/webrtc/v4"
)

// SendToUserFunc sends raw JSON data to a specific connected user.
type SendToUserFunc func(userID uuid.UUID, data []byte)

// Manager manages all active voice sessions across channels.
type Manager struct {
	mu         sync.RWMutex
	sessions   map[uuid.UUID]*VoiceSession
	onEvent    func(data []byte) // broadcasts to all WS clients
	sendToUser SendToUserFunc
	stopSweep  chan struct{}
	stopOnce   sync.Once
}

// NewManager creates a voice manager.
// onEvent is called to broadcast voice state updates to all connected clients.
// sendToUser sends data to a specific user's WebSocket connection.
func NewManager(onEvent func([]byte), sendToUser SendToUserFunc) *Manager {
	return &Manager{
		sessions:   make(map[uuid.UUID]*VoiceSession),
		onEvent:    onEvent,
		sendToUser: sendToUser,
		stopSweep:  make(chan struct{}),
	}
}

// StartSweeper runs a periodic goroutine that evicts participants whose
// PeerConnection is in failed/closed/disconnected state.
func (m *Manager) StartSweeper() {
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				m.sweepStale()
			case <-m.stopSweep:
				return
			}
		}
	}()
}

// StopSweeper stops the periodic stale session sweeper.
func (m *Manager) StopSweeper() {
	m.stopOnce.Do(func() {
		close(m.stopSweep)
	})
}

func (m *Manager) sweepStale() {
	m.mu.RLock()
	sessions := make(map[uuid.UUID]*VoiceSession, len(m.sessions))
	for id, s := range m.sessions {
		sessions[id] = s
	}
	m.mu.RUnlock()

	for channelID, session := range sessions {
		session.mu.RLock()
		var staleUsers []uuid.UUID
		for uid, p := range session.participants {
			state := p.PC.ConnectionState()
			if state == webrtc.PeerConnectionStateFailed ||
				state == webrtc.PeerConnectionStateClosed {
				staleUsers = append(staleUsers, uid)
			}
		}
		session.mu.RUnlock()

		for _, uid := range staleUsers {
			log.Printf("voice: sweeper evicting stale user %s from channel %s", uid, channelID)
			session.RemoveParticipant(uid)
			m.broadcastVoiceState(channelID)
		}

		if session.IsEmpty() {
			m.mu.Lock()
			if session.IsEmpty() {
				delete(m.sessions, channelID)
			}
			m.mu.Unlock()
		}
	}
}

// Join adds a user to a voice channel, creating the session if needed.
// Returns the SDP offer and transceiver MIDs to send to the client.
func (m *Manager) Join(channelID, userID uuid.UUID, username, avatarPath string) (json.RawMessage, map[string]string, error) {
	log.Printf("voice: Manager.Join called for %s (%s) in channel %s", userID, username, channelID)
	// Disconnect user from any other voice channel first
	m.DisconnectUser(userID)

	m.mu.Lock()
	session, ok := m.sessions[channelID]
	if !ok {
		session = NewVoiceSession(channelID, m.sendToUser, m.onEvent, func(removedUserID uuid.UUID) {
			m.broadcastVoiceState(channelID)
			if session.IsEmpty() {
				m.mu.Lock()
				if session.IsEmpty() {
					delete(m.sessions, channelID)
				}
				m.mu.Unlock()
			}
		})
		m.sessions[channelID] = session
	}
	m.mu.Unlock()

	result, err := session.AddParticipant(userID, username, avatarPath)
	if err != nil {
		return nil, nil, err
	}

	m.broadcastVoiceState(channelID)
	return result.SDP, result.TransceiverMIDs, nil
}

// Leave removes a user from a voice channel.
func (m *Manager) Leave(channelID, userID uuid.UUID) {
	m.mu.RLock()
	session, ok := m.sessions[channelID]
	m.mu.RUnlock()
	if !ok {
		return
	}

	session.RemoveParticipant(userID)
	m.broadcastVoiceState(channelID)

	// Clean up empty sessions
	if session.IsEmpty() {
		m.mu.Lock()
		if session.IsEmpty() {
			delete(m.sessions, channelID)
		}
		m.mu.Unlock()
	}
}

// HandleAnswer passes an SDP answer to the correct session.
func (m *Manager) HandleAnswer(channelID, userID uuid.UUID, sdp json.RawMessage) error {
	m.mu.RLock()
	session, ok := m.sessions[channelID]
	m.mu.RUnlock()
	if !ok {
		return nil
	}
	return session.HandleAnswer(userID, sdp)
}

// HandleICE passes an ICE candidate to the correct session.
func (m *Manager) HandleICE(channelID, userID uuid.UUID, candidate json.RawMessage) error {
	m.mu.RLock()
	session, ok := m.sessions[channelID]
	m.mu.RUnlock()
	if !ok {
		return nil
	}
	return session.HandleICE(userID, candidate)
}

// SetMuted sets the muted state for a user in a channel.
func (m *Manager) SetMuted(channelID, userID uuid.UUID, muted bool) {
	m.mu.RLock()
	session, ok := m.sessions[channelID]
	m.mu.RUnlock()
	if !ok {
		return
	}
	session.SetMuted(userID, muted)
	m.broadcastVoiceState(channelID)
}

// SetDeafened sets the deafened state for a user in a channel.
func (m *Manager) SetDeafened(channelID, userID uuid.UUID, deafened bool) {
	m.mu.RLock()
	session, ok := m.sessions[channelID]
	m.mu.RUnlock()
	if !ok {
		return
	}
	session.SetDeafened(userID, deafened)
	m.broadcastVoiceState(channelID)
}

// DisconnectUser removes a user from all voice sessions (called on WS disconnect).
func (m *Manager) DisconnectUser(userID uuid.UUID) {
	log.Printf("voice: DisconnectUser called for %s", userID)
	m.mu.RLock()
	sessions := make(map[uuid.UUID]*VoiceSession, len(m.sessions))
	for id, s := range m.sessions {
		sessions[id] = s
	}
	m.mu.RUnlock()

	for channelID, session := range sessions {
		session.RemoveParticipant(userID)
		m.broadcastVoiceState(channelID)
		if session.IsEmpty() {
			m.mu.Lock()
			if session.IsEmpty() {
				delete(m.sessions, channelID)
			}
			m.mu.Unlock()
		}
	}
}

// GetAllVoiceStates returns the current voice state for all active channels.
func (m *Manager) GetAllVoiceStates() map[uuid.UUID][]ParticipantState {
	m.mu.RLock()
	defer m.mu.RUnlock()

	states := make(map[uuid.UUID][]ParticipantState, len(m.sessions))
	for channelID, session := range m.sessions {
		participants := session.Participants()
		if len(participants) > 0 {
			states[channelID] = participants
		}
	}
	return states
}

// StartScreenShare starts screen sharing for a user in a channel.
// Returns true if successful, false if someone else is already sharing.
func (m *Manager) StartScreenShare(channelID, userID uuid.UUID) bool {
	m.mu.RLock()
	session, ok := m.sessions[channelID]
	m.mu.RUnlock()
	if !ok {
		return false
	}
	if session.StartScreenShare(userID) {
		m.broadcastVoiceState(channelID)
		return true
	}
	return false
}

// StopScreenShare stops screen sharing for a user in a channel.
func (m *Manager) StopScreenShare(channelID, userID uuid.UUID) {
	m.mu.RLock()
	session, ok := m.sessions[channelID]
	m.mu.RUnlock()
	if !ok {
		return
	}
	session.StopScreenShare(userID)
	m.broadcastVoiceState(channelID)
}

// StartCamera starts camera for a user in a channel.
func (m *Manager) StartCamera(channelID, userID uuid.UUID) {
	m.mu.RLock()
	session, ok := m.sessions[channelID]
	m.mu.RUnlock()
	if !ok {
		return
	}
	session.StartCamera(userID)
	m.broadcastVoiceState(channelID)
}

// StopCamera stops camera for a user in a channel.
func (m *Manager) StopCamera(channelID, userID uuid.UUID) {
	m.mu.RLock()
	session, ok := m.sessions[channelID]
	m.mu.RUnlock()
	if !ok {
		return
	}
	session.StopCamera(userID)
	m.broadcastVoiceState(channelID)
}

// broadcastVoiceState sends the current voice state for a channel to all clients.
func (m *Manager) broadcastVoiceState(channelID uuid.UUID) {
	m.mu.RLock()
	session, ok := m.sessions[channelID]
	m.mu.RUnlock()

	var participants []ParticipantState
	if ok {
		participants = session.Participants()
	}
	if participants == nil {
		participants = []ParticipantState{}
	}

	data, err := json.Marshal(map[string]interface{}{
		"type":         "voice_state",
		"channel_id":   channelID.String(),
		"participants": participants,
	})
	if err != nil {
		log.Printf("voice: broadcast state marshal error: %v", err)
		return
	}
	m.onEvent(data)
}
