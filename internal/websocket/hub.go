package websocket

import (
	"encoding/json"
	"log"
	"sync"

	"github.com/google/uuid"
)

// Hub maintains channel-scoped client subscriptions and broadcasts messages
// to subscribers of specific channels.
type Hub struct {
	// channels maps channelID -> set of subscribed clients
	channels map[uuid.UUID]map[*Client]struct{}

	// allClients tracks every connected client for global broadcasts (e.g. presence).
	allClients map[*Client]struct{}

	presence *PresenceTracker

	register   chan *Client
	unregister chan *Client

	subscribe   chan subscribeRequest
	unsubscribe chan subscribeRequest

	broadcast chan broadcastRequest

	mu sync.RWMutex
}

type subscribeRequest struct {
	client    *Client
	channelID uuid.UUID
}

type broadcastRequest struct {
	channelID uuid.UUID
	data      []byte
}

func NewHub() *Hub {
	return &Hub{
		channels:    make(map[uuid.UUID]map[*Client]struct{}),
		allClients:  make(map[*Client]struct{}),
		presence:    NewPresenceTracker(),
		register:    make(chan *Client),
		unregister:  make(chan *Client),
		subscribe:   make(chan subscribeRequest),
		unsubscribe: make(chan subscribeRequest),
		broadcast:   make(chan broadcastRequest, 256),
	}
}

// Presence returns the hub's presence tracker (used by REST handlers).
func (h *Hub) Presence() *PresenceTracker {
	return h.presence
}

// Run starts the hub event loop. Should be called in its own goroutine.
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			client.hub = h
			h.allClients[client] = struct{}{}
			h.mu.Unlock()
			if h.presence.SetOnline(client.UserID, client) {
				h.broadcastPresence(client.UserID, "online")
			}

		case client := <-h.unregister:
			h.mu.Lock()
			h.removeClientLocked(client)
			delete(h.allClients, client)
			h.mu.Unlock()
			if h.presence.SetOffline(client.UserID, client) {
				h.broadcastPresence(client.UserID, "offline")
			}

		case req := <-h.subscribe:
			h.mu.Lock()
			subs, ok := h.channels[req.channelID]
			if !ok {
				subs = make(map[*Client]struct{})
				h.channels[req.channelID] = subs
			}
			subs[req.client] = struct{}{}
			h.mu.Unlock()

		case req := <-h.unsubscribe:
			h.mu.Lock()
			if subs, ok := h.channels[req.channelID]; ok {
				delete(subs, req.client)
				if len(subs) == 0 {
					delete(h.channels, req.channelID)
				}
			}
			h.mu.Unlock()

		case req := <-h.broadcast:
			h.mu.RLock()
			subs := h.channels[req.channelID]
			for client := range subs {
				select {
				case client.send <- req.data:
				default:
					// slow client — drop and clean up
					go func(c *Client) {
						h.unregister <- c
					}(client)
				}
			}
			h.mu.RUnlock()
		}
	}
}

// Subscribe adds a client to a channel's subscriber set.
func (h *Hub) Subscribe(client *Client, channelID uuid.UUID) {
	h.subscribe <- subscribeRequest{client: client, channelID: channelID}
}

// Unsubscribe removes a client from a channel's subscriber set.
func (h *Hub) Unsubscribe(client *Client, channelID uuid.UUID) {
	h.unsubscribe <- subscribeRequest{client: client, channelID: channelID}
}

// UnsubscribeAll removes a client from every channel and closes its send buffer.
func (h *Hub) UnsubscribeAll(client *Client) {
	h.unregister <- client
}

// BroadcastToChannel sends data to all clients subscribed to the given channel.
func (h *Hub) BroadcastToChannel(channelID uuid.UUID, data []byte) {
	h.broadcast <- broadcastRequest{channelID: channelID, data: data}
}

// Register adds a client to the hub.
func (h *Hub) Register(client *Client) {
	h.register <- client
}

// broadcastPresence sends a presence_update to all connected clients.
func (h *Hub) broadcastPresence(userID uuid.UUID, status string) {
	data, err := json.Marshal(map[string]string{
		"type":    "presence_update",
		"user_id": userID.String(),
		"status":  status,
	})
	if err != nil {
		log.Printf("presence marshal error: %v", err)
		return
	}
	h.mu.RLock()
	for client := range h.allClients {
		select {
		case client.send <- data:
		default:
		}
	}
	h.mu.RUnlock()
}

// SendToClient sends raw JSON data to a single client.
func (h *Hub) SendToClient(client *Client, data []byte) {
	select {
	case client.send <- data:
	default:
	}
}

// removeClientLocked removes a client from all channels. Caller must hold h.mu write lock.
func (h *Hub) removeClientLocked(client *Client) {
	for chID, subs := range h.channels {
		delete(subs, client)
		if len(subs) == 0 {
			delete(h.channels, chID)
		}
	}
	client.CloseSend()
}
