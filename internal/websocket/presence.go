package websocket

import (
	"sync"

	"github.com/google/uuid"
)

// PresenceTracker tracks which users are online via their WebSocket connections.
type PresenceTracker struct {
	mu    sync.RWMutex
	users map[uuid.UUID]map[*Client]struct{} // userID → set of clients
}

func NewPresenceTracker() *PresenceTracker {
	return &PresenceTracker{
		users: make(map[uuid.UUID]map[*Client]struct{}),
	}
}

// SetOnline adds a client to the user's connection set.
// Returns true if this is the user's first connection (status changed to online).
func (p *PresenceTracker) SetOnline(userID uuid.UUID, client *Client) bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	clients, exists := p.users[userID]
	if !exists {
		clients = make(map[*Client]struct{})
		p.users[userID] = clients
	}
	clients[client] = struct{}{}
	return !exists
}

// SetOffline removes a client from the user's connection set.
// Returns true if this was the user's last connection (status changed to offline).
func (p *PresenceTracker) SetOffline(userID uuid.UUID, client *Client) bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	clients, exists := p.users[userID]
	if !exists {
		return false
	}
	delete(clients, client)
	if len(clients) == 0 {
		delete(p.users, userID)
		return true
	}
	return false
}

// IsOnline returns true if the user has at least one active connection.
func (p *PresenceTracker) IsOnline(userID uuid.UUID) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.users[userID]) > 0
}

// OnlineUserIDs returns all user IDs with at least one active connection.
func (p *PresenceTracker) OnlineUserIDs() []uuid.UUID {
	p.mu.RLock()
	defer p.mu.RUnlock()

	ids := make([]uuid.UUID, 0, len(p.users))
	for id := range p.users {
		ids = append(ids, id)
	}
	return ids
}
