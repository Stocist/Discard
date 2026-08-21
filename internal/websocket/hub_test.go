package websocket

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
)

func testClient(userID uuid.UUID) *Client {
	return &Client{UserID: userID, send: make(chan []byte, 8)}
}

func TestOnlineUserIDsForFiltersBlockedUsers(t *testing.T) {
	hub := NewHub()
	viewerID := uuid.New()
	visibleID := uuid.New()
	blockedID := uuid.New()
	hub.presence.SetOnline(visibleID, testClient(visibleID))
	hub.presence.SetOnline(blockedID, testClient(blockedID))
	hub.SetPresenceVisibilityChecker(func(_ context.Context, viewer, subject uuid.UUID) (bool, error) {
		return viewer != viewerID || subject != blockedID, nil
	})

	visible := hub.OnlineUserIDsFor(context.Background(), viewerID)
	if len(visible) != 1 || visible[0] != visibleID {
		t.Fatalf("visible IDs = %v, want [%s]", visible, visibleID)
	}
}

func TestNotifyBlockStateTargetsBothUsers(t *testing.T) {
	hub := NewHub()
	userA := uuid.New()
	userB := uuid.New()
	clientA := testClient(userA)
	clientB := testClient(userB)
	hub.allClients[clientA] = struct{}{}
	hub.allClients[clientB] = struct{}{}
	hub.presence.SetOnline(userA, clientA)
	hub.presence.SetOnline(userB, clientB)

	hub.NotifyBlockState(userA, userB, true, false)

	assertBlockEvents(t, clientA.send, userB, true, true)
	assertBlockEvents(t, clientB.send, userA, true, false)
}

func assertBlockEvents(t *testing.T, events <-chan []byte, otherID uuid.UUID, blockedEither, blockedByMe bool) {
	t.Helper()
	var state struct {
		Type          string `json:"type"`
		UserID        string `json:"user_id"`
		BlockedEither bool   `json:"blocked_either"`
		BlockedByMe   bool   `json:"blocked_by_me"`
	}
	if err := json.Unmarshal(<-events, &state); err != nil {
		t.Fatal(err)
	}
	if state.Type != "block_state" || state.UserID != otherID.String() || state.BlockedEither != blockedEither || state.BlockedByMe != blockedByMe {
		t.Fatalf("unexpected block state: %+v", state)
	}
	var presence struct {
		Type   string `json:"type"`
		UserID string `json:"user_id"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(<-events, &presence); err != nil {
		t.Fatal(err)
	}
	if presence.Type != "presence_update" || presence.UserID != otherID.String() || presence.Status != "offline" {
		t.Fatalf("unexpected presence update: %+v", presence)
	}
}
