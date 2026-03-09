package voice

import (
	"encoding/json"
	"log"
	"sync"

	"github.com/google/uuid"
)

// SendToUserFunc sends raw JSON data to a specific connected user.
type SendToUserFunc func(userID uuid.UUID, data []byte)

// Manager manages all active voice sessions across channels.
type Manager struct {
	mu         sync.RWMutex
	sessions   map[uuid.UUID]*VoiceSession
	onEvent    func(data []byte) // broadcasts to all WS clients
	sendToUser SendToUserFunc
}

// NewManager creates a voice manager.
// onEvent is called to broadcast voice state updates to all connected clients.
// sendToUser sends data to a specific user's WebSocket connection.
func NewManager(onEvent func([]byte), sendToUser SendToUserFunc) *Manager {
	return &Manager{
		sessions:   make(map[uuid.UUID]*VoiceSession),
		onEvent:    onEvent,
		sendToUser: sendToUser,
	}
}

// Join adds a user to a voice channel, creating the session if needed.
// Returns an SDP offer as JSON to send to the client.
func (m *Manager) Join(channelID, userID uuid.UUID, username, avatarPath string) (json.RawMessage, error) {
	// Disconnect user from any other voice channel first
	m.DisconnectUser(userID)

	m.mu.Lock()
	session, ok := m.sessions[channelID]
	if !ok {
		session = NewVoiceSession(channelID, m.sendToUser, m.onEvent)
		m.sessions[channelID] = session
	}
	m.mu.Unlock()

	sdp, err := session.AddParticipant(userID, username, avatarPath)
	if err != nil {
		return nil, err
	}

	m.broadcastVoiceState(channelID)
	return sdp, nil
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
	m.mu.RLock()
	sessions := make(map[uuid.UUID]*VoiceSession, len(m.sessions))
	for id, s := range m.sessions {
		sessions[id] = s
	}
	m.mu.RUnlock()

	for channelID, session := range sessions {
		session.RemoveParticipant(userID)
		if session.IsEmpty() {
			m.mu.Lock()
			if session.IsEmpty() {
				delete(m.sessions, channelID)
			}
			m.mu.Unlock()
		} else {
			m.broadcastVoiceState(channelID)
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
