package database

import (
	"context"
	"database/sql"
	"time"

	"github.com/Stocist/discard/internal/models"
	"github.com/google/uuid"
)

// AttachmentRepo handles attachment-related database operations.
type AttachmentRepo struct {
	DB *sql.DB
}

func (r *AttachmentRepo) Create(ctx context.Context, a *models.Attachment) error {
	a.ID = uuid.New()
	a.CreatedAt = time.Now()
	_, err := r.DB.ExecContext(ctx,
		`INSERT INTO attachments (id, message_id, file_path, original_name, mime_type, file_size, width, height, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		a.ID, a.MessageID, a.FilePath, a.OriginalName, a.MimeType, a.FileSize, a.Width, a.Height, a.CreatedAt,
	)
	return err
}

func (r *AttachmentRepo) ListByMessage(ctx context.Context, messageID uuid.UUID) ([]models.Attachment, error) {
	rows, err := r.DB.QueryContext(ctx,
		`SELECT id, message_id, file_path, original_name, mime_type, file_size, width, height, created_at
		 FROM attachments WHERE message_id = $1
		 ORDER BY created_at`, messageID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var attachments []models.Attachment
	for rows.Next() {
		var a models.Attachment
		if err := rows.Scan(&a.ID, &a.MessageID, &a.FilePath, &a.OriginalName, &a.MimeType, &a.FileSize, &a.Width, &a.Height, &a.CreatedAt); err != nil {
			return nil, err
		}
		attachments = append(attachments, a)
	}
	return attachments, rows.Err()
}
