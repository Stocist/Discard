package websocket

import (
	"context"
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"

	"github.com/Stocist/discard/internal/models"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	maxMessageSize = 4096
	sendBufSize    = 256
)

// MessageHandler persists a chat message and returns the saved model.
type MessageHandler func(ctx context.Context, channelID uuid.UUID, authorID uuid.UUID, content string) (*models.Message, error)

// MembershipChecker verifies a user belongs to a channel before subscribing.
type MembershipChecker func(ctx context.Context, userID uuid.UUID, channelID uuid.UUID) (bool, error)

// Client is a middleman between a WebSocket connection and the Hub.
type Client struct {
	hub      *Hub
	conn     *websocket.Conn
	send     chan []byte
	closeSend sync.Once
	UserID   uuid.UUID

	// OnMessage is called to persist incoming chat messages.
	OnMessage MessageHandler

	// CheckMembership is called before subscribing to a channel.
	CheckMembership MembershipChecker
}

// CloseSend safely closes the send channel exactly once.
func (c *Client) CloseSend() {
	c.closeSend.Do(func() {
		close(c.send)
	})
}

// NewClient creates a Client. Call Hub.Register(client) after creation.
func NewClient(conn *websocket.Conn, userID uuid.UUID, handler MessageHandler, checker MembershipChecker) *Client {
	return &Client{
		conn:            conn,
		send:            make(chan []byte, sendBufSize),
		UserID:          userID,
		OnMessage:       handler,
		CheckMembership: checker,
	}
}

// incomingMessage is the envelope for messages from the browser.
type incomingMessage struct {
	Type      string    `json:"type"`
	ChannelID uuid.UUID `json:"channel_id"`
	Content   string    `json:"content,omitempty"`
}

// outgoingMessage is the envelope for messages sent to the browser.
type outgoingMessage struct {
	Type    string          `json:"type"`
	Message *models.Message `json:"message,omitempty"`
}

// ReadPump pumps messages from the WebSocket to the hub.
// Must be called in its own goroutine — one per connection.
func (c *Client) ReadPump() {
	defer func() {
		c.hub.UnsubscribeAll(c)
		c.conn.Close()
	}()

	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, raw, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				log.Printf("ws read error: %v", err)
			}
			break
		}

		var msg incomingMessage
		if err := json.Unmarshal(raw, &msg); err != nil {
			log.Printf("ws unmarshal error: %v", err)
			continue
		}

		switch msg.Type {
		case "subscribe":
			if c.CheckMembership != nil {
				ok, err := c.CheckMembership(context.Background(), c.UserID, msg.ChannelID)
				if err != nil {
					log.Printf("ws membership check error: %v", err)
					c.sendError("failed to verify channel membership")
					continue
				}
				if !ok {
					c.sendError("not a member of this channel")
					continue
				}
			}
			c.hub.Subscribe(c, msg.ChannelID)

		case "unsubscribe":
			c.hub.Unsubscribe(c, msg.ChannelID)

		case "message":
			c.handleChatMessage(msg)

		default:
			log.Printf("ws unknown message type: %s", msg.Type)
		}
	}
}

// WritePump pumps messages from the hub to the WebSocket connection.
// Must be called in its own goroutine — one per connection.
func (c *Client) WritePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case data, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, nil)
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, data); err != nil {
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// sendError writes a JSON error message to the client's WebSocket.
func (c *Client) sendError(message string) {
	out, err := json.Marshal(map[string]string{"type": "error", "message": message})
	if err != nil {
		log.Printf("ws marshal error: %v", err)
		return
	}
	select {
	case c.send <- out:
	default:
		log.Printf("ws send buffer full, dropping error message")
	}
}

func (c *Client) handleChatMessage(msg incomingMessage) {
	if msg.Content == "" || c.OnMessage == nil {
		return
	}
	if len(msg.Content) > 4000 {
		log.Printf("ws message too long from user %s: %d chars", c.UserID, len(msg.Content))
		return
	}

	saved, err := c.OnMessage(context.Background(), msg.ChannelID, c.UserID, msg.Content)
	if err != nil {
		log.Printf("ws message handler error: %v", err)
		return
	}

	out, err := json.Marshal(outgoingMessage{
		Type:    "message",
		Message: saved,
	})
	if err != nil {
		log.Printf("ws marshal error: %v", err)
		return
	}

	c.hub.BroadcastToChannel(msg.ChannelID, out)
}
