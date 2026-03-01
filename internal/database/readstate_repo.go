package database

import (
	"context"
	"database/sql"

	"github.com/google/uuid"
)

// ReadStateRepo handles channel read state operations.
type ReadStateRepo struct {
	DB *sql.DB
}

func (r *ReadStateRepo) UpdateReadState(ctx context.Context, userID, channelID uuid.UUID, messageID *uuid.UUID) error {
	_, err := r.DB.ExecContext(ctx,
		`INSERT INTO channel_read_state (user_id, channel_id, last_read_message_id, last_read_at)
		 VALUES ($1, $2, $3, NOW())
		 ON CONFLICT (user_id, channel_id) DO UPDATE
		 SET last_read_message_id = EXCLUDED.last_read_message_id, last_read_at = EXCLUDED.last_read_at`,
		userID, channelID, messageID,
	)
	return err
}

// GetUnreadCounts returns a map of channel_id -> unread count for all channels in a server.
func (r *ReadStateRepo) GetUnreadCounts(ctx context.Context, userID, serverID uuid.UUID) (map[string]int, error) {
	rows, err := r.DB.QueryContext(ctx,
		`SELECT c.id, COUNT(m.id)
		 FROM channels c
		 LEFT JOIN channel_read_state rs ON rs.channel_id = c.id AND rs.user_id = $1
		 LEFT JOIN messages m ON m.channel_id = c.id
		   AND (rs.last_read_message_id IS NULL OR m.created_at > (SELECT created_at FROM messages WHERE id = rs.last_read_message_id))
		 WHERE c.server_id = $2
		 GROUP BY c.id`,
		userID, serverID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	counts := make(map[string]int)
	for rows.Next() {
		var channelID uuid.UUID
		var count int
		if err := rows.Scan(&channelID, &count); err != nil {
			return nil, err
		}
		if count > 0 {
			counts[channelID.String()] = count
		}
	}
	return counts, rows.Err()
}

// GetLatestMessageID returns the ID of the most recent message in a channel, or nil if empty.
func (r *ReadStateRepo) GetLatestMessageID(ctx context.Context, channelID uuid.UUID) (*uuid.UUID, error) {
	var id uuid.UUID
	err := r.DB.QueryRowContext(ctx,
		`SELECT id FROM messages WHERE channel_id = $1 ORDER BY created_at DESC LIMIT 1`, channelID,
	).Scan(&id)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &id, nil
}
