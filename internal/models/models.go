package models

import (
	"time"

	"github.com/google/uuid"
)

type User struct {
	ID           uuid.UUID `json:"id"`
	Username     string    `json:"username"`
	DisplayName  *string   `json:"display_name"`
	AvatarPath   *string   `json:"avatar_path"`
	TailscaleID  *string   `json:"tailscale_id"`
	PasswordHash *string   `json:"-"`
	Status       string    `json:"status"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

type Server struct {
	ID         uuid.UUID `json:"id"`
	Name       string    `json:"name"`
	IconPath   *string   `json:"icon_path"`
	OwnerID    uuid.UUID `json:"owner_id"`
	InviteCode *string   `json:"invite_code"`
	CreatedAt  time.Time `json:"created_at"`
}

type Channel struct {
	ID        uuid.UUID  `json:"id"`
	ServerID  *uuid.UUID `json:"server_id"`
	Name      *string    `json:"name"`
	Topic     *string    `json:"topic"`
	Type      string     `json:"type"`
	Position  int        `json:"position"`
	CreatedAt time.Time  `json:"created_at"`
}

type Message struct {
	ID             uuid.UUID `json:"id"`
	ChannelID      uuid.UUID `json:"channel_id"`
	AuthorID       uuid.UUID `json:"author_id"`
	Content        string    `json:"content"`
	Edited         bool      `json:"edited"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	AuthorUsername string    `json:"author_username,omitempty"`
}

type Attachment struct {
	ID           uuid.UUID `json:"id"`
	MessageID    uuid.UUID `json:"message_id"`
	FilePath     string    `json:"file_path"`
	OriginalName string    `json:"original_name"`
	MimeType     *string   `json:"mime_type"`
	FileSize     *int64    `json:"file_size"`
	Width        *int      `json:"width"`
	Height       *int      `json:"height"`
	CreatedAt    time.Time `json:"created_at"`
}

type ServerMember struct {
	UserID   uuid.UUID `json:"user_id"`
	ServerID uuid.UUID `json:"server_id"`
	Nickname *string   `json:"nickname"`
	JoinedAt time.Time `json:"joined_at"`
}

type Friendship struct {
	ID          uuid.UUID  `json:"id"`
	UserA       uuid.UUID  `json:"user_a"`
	UserB       uuid.UUID  `json:"user_b"`
	Status      string     `json:"status"`
	InitiatedBy uuid.UUID  `json:"initiated_by"`
	DMChannelID *uuid.UUID `json:"dm_channel_id"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type DMMember struct {
	ChannelID uuid.UUID `json:"channel_id"`
	UserID    uuid.UUID `json:"user_id"`
	JoinedAt  time.Time `json:"joined_at"`
}
