package websocket

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"

	"github.com/Stocist/discard/internal/models"
)

var ErrMessageForbidden = errors.New("message forbidden")

const (
	writeWait  = 10 * time.Second
	pongWait   = 60 * time.Second
	pingPeriod = (pongWait * 9) / 10
	// WebRTC offer/answer SDPs carry one m-line per pre-allocated transceiver
	// per participant, so signaling messages grow well past a few KB with
	// multiple users (an 8-m-line answer is ~5-6 KB). Gorilla closes the
	// connection if an inbound message exceeds this, so it must comfortably
	// fit the largest SDP for a full voice channel.
	maxMessageSize = 1 << 20 // 1 MiB
	sendBufSize    = 256
)

// MessageHandler persists a chat message and returns the saved model.
type MessageHandler func(ctx context.Context, channelID uuid.UUID, authorID uuid.UUID, content string) (*models.Message, error)

// MembershipChecker verifies a user belongs to a channel before subscribing.
type MembershipChecker func(ctx context.Context, userID uuid.UUID, channelID uuid.UUID) (bool, error)

// MessagePermissionChecker verifies that a member may currently send to a channel.
type MessagePermissionChecker func(ctx context.Context, userID uuid.UUID, channelID uuid.UUID) (bool, error)

// Client is a middleman between a WebSocket connection and the Hub.
type Client struct {
	hub        *Hub
	conn       *websocket.Conn
	send       chan []byte
	closeSend  sync.Once
	UserID     uuid.UUID
	Username   string
	AvatarPath string

	// OnMessage is called to persist incoming chat messages.
	OnMessage MessageHandler

	// CheckMembership is called before subscribing to a channel.
	CheckMembership MembershipChecker
	CanSendMessage  MessagePermissionChecker

	// OnVoice handles voice channel signaling.
	OnVoice VoiceHandler
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

// VoiceHandler handles voice signaling without importing the voice package.
type VoiceHandler interface {
	Join(channelID, userID uuid.UUID, username, avatarPath string) (sdp json.RawMessage, transceiverMIDs map[string]string, err error)
	Leave(channelID, userID uuid.UUID)
	HandleAnswer(channelID, userID uuid.UUID, sdp json.RawMessage) error
	HandleICE(channelID, userID uuid.UUID, candidate json.RawMessage) error
	SetMuted(channelID, userID uuid.UUID, muted bool)
	SetDeafened(channelID, userID uuid.UUID, deafened bool)
	DisconnectUser(userID uuid.UUID)
	GetAllVoiceStates() map[uuid.UUID][]VoiceParticipantState
	StartScreenShare(channelID, userID uuid.UUID) bool
	StopScreenShare(channelID, userID uuid.UUID)
	StartCamera(channelID, userID uuid.UUID)
	StopCamera(channelID, userID uuid.UUID)
}

// VoiceParticipantState is the WS-package view of a voice participant.
type VoiceParticipantState struct {
	UserID        uuid.UUID `json:"user_id"`
	Username      string    `json:"username"`
	AvatarPath    string    `json:"avatar_path,omitempty"`
	Muted         bool      `json:"muted"`
	Deafened      bool      `json:"deafened"`
	ScreenSharing bool      `json:"screen_sharing"`
	CameraOn      bool      `json:"camera_on"`
}

// incomingMessage is the envelope for messages from the browser.
type incomingMessage struct {
	Type      string          `json:"type"`
	ChannelID string          `json:"channel_id,omitempty"`
	Content   string          `json:"content,omitempty"`
	SDP       json.RawMessage `json:"sdp,omitempty"`
	Candidate json.RawMessage `json:"candidate,omitempty"`
	Muted     *bool           `json:"muted,omitempty"`
	Deafened  *bool           `json:"deafened,omitempty"`
	Speaking  *bool           `json:"speaking,omitempty"`
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
		// Voice cleanup is NOT done here. It's handled by:
		// 1. Manager.Join → DisconnectUser (cleans stale sessions before new join)
		// 2. OnConnectionStateChange → RemoveParticipant (handles failed PCs)
		// 3. Sweeper (catches dead connections every 30s)
		// Calling DisconnectUser here races with async hub registration and
		// kills voice sessions that a replacement WS connection just created.
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
			channelID, err := uuid.Parse(msg.ChannelID)
			if err != nil {
				c.sendError("invalid channel_id")
				continue
			}
			if c.CheckMembership != nil {
				ok, err := c.CheckMembership(context.Background(), c.UserID, channelID)
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
			c.hub.Subscribe(c, channelID)

		case "unsubscribe":
			channelID, err := uuid.Parse(msg.ChannelID)
			if err != nil {
				continue
			}
			c.hub.Unsubscribe(c, channelID)

		case "message":
			c.handleChatMessage(msg)

		case "presence_request":
			c.handlePresenceRequest()

		case "voice_join":
			c.handleVoiceJoin(msg)
		case "voice_leave":
			c.handleVoiceLeave(msg)
		case "voice_answer":
			c.handleVoiceAnswer(msg)
		case "voice_ice_candidate":
			c.handleVoiceICE(msg)
		case "voice_mute":
			c.handleVoiceMute(msg)
		case "voice_deafen":
			c.handleVoiceDeafen(msg)
		case "voice_speaking":
			c.handleVoiceSpeaking(msg)
		case "voice_state_request":
			c.handleVoiceStateRequest()
		case "screen_share_start":
			c.handleScreenShareStart(msg)
		case "screen_share_stop":
			c.handleScreenShareStop(msg)
		case "voice_camera_start":
			c.handleCameraStart(msg)
		case "voice_camera_stop":
			c.handleCameraStop(msg)

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

func (c *Client) handlePresenceRequest() {
	ids := c.hub.OnlineUserIDsFor(context.Background(), c.UserID)
	strs := make([]string, len(ids))
	for i, id := range ids {
		strs[i] = id.String()
	}
	data, err := json.Marshal(map[string]interface{}{
		"type":     "presence_list",
		"user_ids": strs,
	})
	if err != nil {
		log.Printf("presence list marshal error: %v", err)
		return
	}
	c.hub.SendToClient(c, data)
}

func (c *Client) handleVoiceJoin(msg incomingMessage) {
	if c.OnVoice == nil {
		return
	}
	channelID, err := uuid.Parse(msg.ChannelID)
	if err != nil {
		c.sendError("invalid channel_id")
		return
	}
	sdp, mids, err := c.OnVoice.Join(channelID, c.UserID, c.Username, c.AvatarPath)
	if err != nil {
		log.Printf("voice join error: %v", err)
		c.sendError("failed to join voice channel")
		return
	}
	out := map[string]interface{}{
		"type":       "voice_offer",
		"channel_id": channelID.String(),
		"sdp":        json.RawMessage(sdp),
	}
	if mids != nil {
		out["transceiver_mids"] = mids
	}
	data, _ := json.Marshal(out)
	c.hub.SendToClient(c, data)
}

func (c *Client) handleVoiceLeave(msg incomingMessage) {
	if c.OnVoice == nil {
		return
	}
	channelID, err := uuid.Parse(msg.ChannelID)
	if err != nil {
		return
	}
	c.OnVoice.Leave(channelID, c.UserID)
}

func (c *Client) handleVoiceAnswer(msg incomingMessage) {
	if c.OnVoice == nil {
		return
	}
	channelID, err := uuid.Parse(msg.ChannelID)
	if err != nil {
		c.sendError("invalid channel_id")
		return
	}
	if err := c.OnVoice.HandleAnswer(channelID, c.UserID, msg.SDP); err != nil {
		log.Printf("voice answer error: %v", err)
	}
}

func (c *Client) handleVoiceICE(msg incomingMessage) {
	if c.OnVoice == nil {
		return
	}
	channelID, err := uuid.Parse(msg.ChannelID)
	if err != nil {
		c.sendError("invalid channel_id")
		return
	}
	if err := c.OnVoice.HandleICE(channelID, c.UserID, msg.Candidate); err != nil {
		log.Printf("voice ICE error: %v", err)
	}
}

func (c *Client) handleVoiceMute(msg incomingMessage) {
	if c.OnVoice == nil || msg.Muted == nil {
		return
	}
	channelID, err := uuid.Parse(msg.ChannelID)
	if err != nil {
		return
	}
	c.OnVoice.SetMuted(channelID, c.UserID, *msg.Muted)
}

func (c *Client) handleVoiceDeafen(msg incomingMessage) {
	if c.OnVoice == nil || msg.Deafened == nil {
		return
	}
	channelID, err := uuid.Parse(msg.ChannelID)
	if err != nil {
		return
	}
	c.OnVoice.SetDeafened(channelID, c.UserID, *msg.Deafened)
}

func (c *Client) handleVoiceSpeaking(msg incomingMessage) {
	if msg.Speaking == nil {
		return
	}
	channelID, err := uuid.Parse(msg.ChannelID)
	if err != nil {
		return
	}
	data, _ := json.Marshal(map[string]interface{}{
		"type":       "voice_speaking",
		"channel_id": channelID.String(),
		"user_id":    c.UserID.String(),
		"speaking":   *msg.Speaking,
	})
	c.hub.BroadcastAll(data)
}

func (c *Client) handleVoiceStateRequest() {
	if c.OnVoice == nil {
		return
	}
	states := c.OnVoice.GetAllVoiceStates()

	// Convert uuid keys to strings for JSON
	out := make(map[string]interface{}, len(states))
	for channelID, participants := range states {
		out[channelID.String()] = participants
	}

	data, err := json.Marshal(map[string]interface{}{
		"type":     "voice_state_all",
		"channels": out,
	})
	if err != nil {
		log.Printf("voice state marshal error: %v", err)
		return
	}
	c.hub.SendToClient(c, data)
}

func (c *Client) handleScreenShareStart(msg incomingMessage) {
	if c.OnVoice == nil {
		return
	}
	channelID, err := uuid.Parse(msg.ChannelID)
	if err != nil {
		c.sendError("invalid channel_id")
		return
	}
	ok := c.OnVoice.StartScreenShare(channelID, c.UserID)
	if !ok {
		c.sendError("someone else is already sharing their screen")
		return
	}
	// Broadcast screen share start to all clients
	data, _ := json.Marshal(map[string]interface{}{
		"type":       "screen_share_started",
		"channel_id": channelID.String(),
		"user_id":    c.UserID.String(),
		"username":   c.Username,
	})
	c.hub.BroadcastAll(data)
}

func (c *Client) handleScreenShareStop(msg incomingMessage) {
	if c.OnVoice == nil {
		return
	}
	channelID, err := uuid.Parse(msg.ChannelID)
	if err != nil {
		return
	}
	c.OnVoice.StopScreenShare(channelID, c.UserID)
	// Broadcast screen share stop to all clients
	data, _ := json.Marshal(map[string]interface{}{
		"type":       "screen_share_stopped",
		"channel_id": channelID.String(),
		"user_id":    c.UserID.String(),
	})
	c.hub.BroadcastAll(data)
}

func (c *Client) handleCameraStart(msg incomingMessage) {
	if c.OnVoice == nil {
		return
	}
	channelID, err := uuid.Parse(msg.ChannelID)
	if err != nil {
		c.sendError("invalid channel_id")
		return
	}
	c.OnVoice.StartCamera(channelID, c.UserID)
}

func (c *Client) handleCameraStop(msg incomingMessage) {
	if c.OnVoice == nil {
		return
	}
	channelID, err := uuid.Parse(msg.ChannelID)
	if err != nil {
		return
	}
	c.OnVoice.StopCamera(channelID, c.UserID)
}

func (c *Client) handleChatMessage(msg incomingMessage) {
	if msg.Content == "" || c.OnMessage == nil {
		return
	}
	if len(msg.Content) > 4000 {
		log.Printf("ws message too long from user %s: %d chars", c.UserID, len(msg.Content))
		return
	}
	channelID, err := uuid.Parse(msg.ChannelID)
	if err != nil {
		c.sendError("invalid channel_id")
		return
	}

	if c.CheckMembership != nil {
		ok, err := c.CheckMembership(context.Background(), c.UserID, channelID)
		if err != nil {
			log.Printf("ws membership check error: %v", err)
			c.sendError("failed to verify channel membership")
			return
		}
		if !ok {
			c.sendError("not a member of this channel")
			return
		}
	}
	if c.CanSendMessage != nil {
		ok, err := c.CanSendMessage(context.Background(), c.UserID, channelID)
		if err != nil {
			log.Printf("ws message permission check error: %v", err)
			c.sendError("failed to verify message permission")
			return
		}
		if !ok {
			c.sendError("messaging is disabled for this conversation")
			return
		}
	}

	saved, err := c.OnMessage(context.Background(), channelID, c.UserID, msg.Content)
	if err != nil {
		if errors.Is(err, ErrMessageForbidden) {
			c.sendError("messaging is disabled for this conversation")
			return
		}
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

	c.hub.BroadcastToChannel(channelID, out)
}
