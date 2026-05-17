package voice

import (
	"encoding/json"

	"github.com/google/uuid"

	ws "github.com/Stocist/discard/internal/websocket"
)

// Adapter wraps Manager to satisfy the ws.VoiceHandler interface.
type Adapter struct {
	mgr *Manager
}

// NewAdapter creates an Adapter that bridges voice.Manager to ws.VoiceHandler.
func NewAdapter(mgr *Manager) *Adapter {
	return &Adapter{mgr: mgr}
}

func (a *Adapter) Join(channelID, userID uuid.UUID, username, avatarPath string) (json.RawMessage, map[string]string, error) {
	return a.mgr.Join(channelID, userID, username, avatarPath)
}

func (a *Adapter) Leave(channelID, userID uuid.UUID) {
	a.mgr.Leave(channelID, userID)
}

func (a *Adapter) HandleAnswer(channelID, userID uuid.UUID, sdp json.RawMessage) error {
	return a.mgr.HandleAnswer(channelID, userID, sdp)
}

func (a *Adapter) HandleICE(channelID, userID uuid.UUID, candidate json.RawMessage) error {
	return a.mgr.HandleICE(channelID, userID, candidate)
}

func (a *Adapter) SetMuted(channelID, userID uuid.UUID, muted bool) {
	a.mgr.SetMuted(channelID, userID, muted)
}

func (a *Adapter) SetDeafened(channelID, userID uuid.UUID, deafened bool) {
	a.mgr.SetDeafened(channelID, userID, deafened)
}

func (a *Adapter) DisconnectUser(userID uuid.UUID) {
	a.mgr.DisconnectUser(userID)
}

func (a *Adapter) StartScreenShare(channelID, userID uuid.UUID) bool {
	return a.mgr.StartScreenShare(channelID, userID)
}

func (a *Adapter) StopScreenShare(channelID, userID uuid.UUID) {
	a.mgr.StopScreenShare(channelID, userID)
}

func (a *Adapter) StartCamera(channelID, userID uuid.UUID) {
	a.mgr.StartCamera(channelID, userID)
}

func (a *Adapter) StopCamera(channelID, userID uuid.UUID) {
	a.mgr.StopCamera(channelID, userID)
}

func (a *Adapter) GetAllVoiceStates() map[uuid.UUID][]ws.VoiceParticipantState {
	raw := a.mgr.GetAllVoiceStates()
	result := make(map[uuid.UUID][]ws.VoiceParticipantState, len(raw))
	for channelID, participants := range raw {
		wsParticipants := make([]ws.VoiceParticipantState, len(participants))
		for i, p := range participants {
			wsParticipants[i] = ws.VoiceParticipantState{
				UserID:        p.UserID,
				Username:      p.Username,
				AvatarPath:    p.AvatarPath,
				Muted:         p.Muted,
				Deafened:      p.Deafened,
				ScreenSharing: p.ScreenSharing,
				CameraOn:      p.CameraOn,
			}
		}
		result[channelID] = wsParticipants
	}
	return result
}
