package websocket

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/Stocist/discard/internal/models"
	"github.com/google/uuid"
)

func TestHandleChatMessageRejectsBlockedConversation(t *testing.T) {
	called := false
	client := testClient(uuid.New())
	client.hub = NewHub()
	client.CheckMembership = func(context.Context, uuid.UUID, uuid.UUID) (bool, error) { return true, nil }
	client.CanSendMessage = func(context.Context, uuid.UUID, uuid.UUID) (bool, error) { return false, nil }
	client.OnMessage = func(context.Context, uuid.UUID, uuid.UUID, string) (*models.Message, error) {
		called = true
		return nil, nil
	}

	client.handleChatMessage(incomingMessage{Type: "message", ChannelID: uuid.NewString(), Content: "blocked"})
	if called {
		t.Fatal("message handler was called for a blocked conversation")
	}
	var event map[string]string
	if err := json.Unmarshal(<-client.send, &event); err != nil {
		t.Fatal(err)
	}
	if event["type"] != "error" || event["message"] != "messaging is disabled for this conversation" {
		t.Fatalf("unexpected error event: %v", event)
	}
}

func TestHandleChatMessageAllowsPermittedConversation(t *testing.T) {
	channelID := uuid.New()
	client := testClient(uuid.New())
	client.hub = NewHub()
	client.CheckMembership = func(context.Context, uuid.UUID, uuid.UUID) (bool, error) { return true, nil }
	client.CanSendMessage = func(context.Context, uuid.UUID, uuid.UUID) (bool, error) { return true, nil }
	client.OnMessage = func(_ context.Context, gotChannelID, authorID uuid.UUID, content string) (*models.Message, error) {
		return &models.Message{ID: uuid.New(), ChannelID: gotChannelID, AuthorID: authorID, Content: content}, nil
	}

	client.handleChatMessage(incomingMessage{Type: "message", ChannelID: channelID.String(), Content: "allowed"})
	broadcast := <-client.hub.broadcast
	if broadcast.channelID != channelID {
		t.Fatalf("broadcast channel = %s, want %s", broadcast.channelID, channelID)
	}
}
