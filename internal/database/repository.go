package database

import (
	"context"
	"database/sql"
	"time"

	"github.com/Stocist/discard/internal/models"
	"github.com/google/uuid"
)

// UserRepo handles user-related database operations.
type UserRepo struct {
	DB *sql.DB
}

// Create inserts a new user into the database.
// Implements auth.UserRepo.
func (r *UserRepo) Create(ctx context.Context, u *models.User) error {
	u.ID = uuid.New()
	now := time.Now()
	u.CreatedAt = now
	u.UpdatedAt = now
	_, err := r.DB.ExecContext(ctx,
		`INSERT INTO users (id, username, display_name, avatar_path, tailscale_id, password_hash, status, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		u.ID, u.Username, u.DisplayName, u.AvatarPath, u.TailscaleID, u.PasswordHash, u.Status, u.CreatedAt, u.UpdatedAt,
	)
	return err
}

func (r *UserRepo) GetByID(ctx context.Context, id uuid.UUID) (*models.User, error) {
	u := &models.User{}
	err := r.DB.QueryRowContext(ctx,
		`SELECT id, username, display_name, avatar_path, tailscale_id, password_hash, status, created_at, updated_at
		 FROM users WHERE id = $1`, id,
	).Scan(&u.ID, &u.Username, &u.DisplayName, &u.AvatarPath, &u.TailscaleID, &u.PasswordHash, &u.Status, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return u, nil
}

// GetByTailscaleID looks up a user by their Tailscale user profile identity.
// Implements auth.UserRepo.
func (r *UserRepo) GetByTailscaleID(ctx context.Context, tsID string) (*models.User, error) {
	u := &models.User{}
	err := r.DB.QueryRowContext(ctx,
		`SELECT id, username, display_name, avatar_path, tailscale_id, password_hash, status, created_at, updated_at
		 FROM users WHERE tailscale_id = $1`, tsID,
	).Scan(&u.ID, &u.Username, &u.DisplayName, &u.AvatarPath, &u.TailscaleID, &u.PasswordHash, &u.Status, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return u, nil
}

func (r *UserRepo) UpdateUserStatus(ctx context.Context, id uuid.UUID, status string) error {
	_, err := r.DB.ExecContext(ctx,
		`UPDATE users SET status = $1, updated_at = $2 WHERE id = $3`,
		status, time.Now(), id,
	)
	return err
}

func (r *UserRepo) UpdateProfile(ctx context.Context, id uuid.UUID, displayName *string, avatarPath *string) (*models.User, error) {
	u := &models.User{}
	err := r.DB.QueryRowContext(ctx,
		`UPDATE users SET display_name = COALESCE($2, display_name), avatar_path = COALESCE($3, avatar_path), updated_at = $4
		 WHERE id = $1
		 RETURNING id, username, display_name, avatar_path, tailscale_id, password_hash, status, created_at, updated_at`,
		id, displayName, avatarPath, time.Now(),
	).Scan(&u.ID, &u.Username, &u.DisplayName, &u.AvatarPath, &u.TailscaleID, &u.PasswordHash, &u.Status, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return u, nil
}

// ServerRepo handles server-related database operations.
type ServerRepo struct {
	DB *sql.DB
}

func (r *ServerRepo) CreateServer(ctx context.Context, s *models.Server) error {
	s.ID = uuid.New()
	s.CreatedAt = time.Now()
	_, err := r.DB.ExecContext(ctx,
		`INSERT INTO servers (id, name, icon_path, owner_id, invite_code, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		s.ID, s.Name, s.IconPath, s.OwnerID, s.InviteCode, s.CreatedAt,
	)
	return err
}

func (r *ServerRepo) GetServerByID(ctx context.Context, id uuid.UUID) (*models.Server, error) {
	s := &models.Server{}
	err := r.DB.QueryRowContext(ctx,
		`SELECT id, name, icon_path, owner_id, invite_code, created_at
		 FROM servers WHERE id = $1`, id,
	).Scan(&s.ID, &s.Name, &s.IconPath, &s.OwnerID, &s.InviteCode, &s.CreatedAt)
	if err != nil {
		return nil, err
	}
	return s, nil
}

func (r *ServerRepo) ListUserServers(ctx context.Context, userID uuid.UUID) ([]models.Server, error) {
	rows, err := r.DB.QueryContext(ctx,
		`SELECT s.id, s.name, s.icon_path, s.owner_id, s.invite_code, s.created_at
		 FROM servers s
		 INNER JOIN server_members sm ON s.id = sm.server_id
		 WHERE sm.user_id = $1
		 ORDER BY s.name`, userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var servers []models.Server
	for rows.Next() {
		var s models.Server
		if err := rows.Scan(&s.ID, &s.Name, &s.IconPath, &s.OwnerID, &s.InviteCode, &s.CreatedAt); err != nil {
			return nil, err
		}
		servers = append(servers, s)
	}
	return servers, rows.Err()
}

func (r *ServerRepo) UpdateServer(ctx context.Context, id uuid.UUID, name string, iconPath *string) (*models.Server, error) {
	s := &models.Server{}
	err := r.DB.QueryRowContext(ctx,
		`UPDATE servers SET name = $1, icon_path = COALESCE($3, icon_path) WHERE id = $2
		 RETURNING id, name, icon_path, owner_id, invite_code, created_at`,
		name, id, iconPath,
	).Scan(&s.ID, &s.Name, &s.IconPath, &s.OwnerID, &s.InviteCode, &s.CreatedAt)
	if err != nil {
		return nil, err
	}
	return s, nil
}

func (r *ServerRepo) DeleteServer(ctx context.Context, id uuid.UUID) error {
	result, err := r.DB.ExecContext(ctx, `DELETE FROM servers WHERE id = $1`, id)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// ChannelRepo handles channel-related database operations.
type ChannelRepo struct {
	DB *sql.DB
}

func (r *ChannelRepo) CreateChannel(ctx context.Context, c *models.Channel) error {
	c.ID = uuid.New()
	c.CreatedAt = time.Now()
	_, err := r.DB.ExecContext(ctx,
		`INSERT INTO channels (id, server_id, name, topic, type, position, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		c.ID, c.ServerID, c.Name, c.Topic, c.Type, c.Position, c.CreatedAt,
	)
	return err
}

func (r *ChannelRepo) GetChannelByID(ctx context.Context, id uuid.UUID) (*models.Channel, error) {
	c := &models.Channel{}
	err := r.DB.QueryRowContext(ctx,
		`SELECT id, server_id, name, topic, type, position, created_at
		 FROM channels WHERE id = $1`, id,
	).Scan(&c.ID, &c.ServerID, &c.Name, &c.Topic, &c.Type, &c.Position, &c.CreatedAt)
	if err != nil {
		return nil, err
	}
	return c, nil
}

func (r *ChannelRepo) UpdateChannel(ctx context.Context, channelID uuid.UUID, name string) (*models.Channel, error) {
	c := &models.Channel{}
	err := r.DB.QueryRowContext(ctx,
		`UPDATE channels SET name = $1 WHERE id = $2
		 RETURNING id, server_id, name, topic, type, position, created_at`,
		name, channelID,
	).Scan(&c.ID, &c.ServerID, &c.Name, &c.Topic, &c.Type, &c.Position, &c.CreatedAt)
	if err != nil {
		return nil, err
	}
	return c, nil
}

func (r *ChannelRepo) DeleteChannel(ctx context.Context, channelID uuid.UUID) error {
	result, err := r.DB.ExecContext(ctx,
		`DELETE FROM channels WHERE id = $1`, channelID,
	)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *ChannelRepo) ListServerChannels(ctx context.Context, serverID uuid.UUID) ([]models.Channel, error) {
	rows, err := r.DB.QueryContext(ctx,
		`SELECT id, server_id, name, topic, type, position, created_at
		 FROM channels WHERE server_id = $1
		 ORDER BY position, created_at`, serverID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var channels []models.Channel
	for rows.Next() {
		var c models.Channel
		if err := rows.Scan(&c.ID, &c.ServerID, &c.Name, &c.Topic, &c.Type, &c.Position, &c.CreatedAt); err != nil {
			return nil, err
		}
		channels = append(channels, c)
	}
	return channels, rows.Err()
}

// ServerMemberRepo handles server membership operations.
type ServerMemberRepo struct {
	DB *sql.DB
}

func (r *ServerMemberRepo) AddMember(ctx context.Context, m *models.ServerMember) error {
	m.JoinedAt = time.Now()
	_, err := r.DB.ExecContext(ctx,
		`INSERT INTO server_members (user_id, server_id, nickname, joined_at)
		 VALUES ($1, $2, $3, $4)`,
		m.UserID, m.ServerID, m.Nickname, m.JoinedAt,
	)
	return err
}

func (r *ServerMemberRepo) RemoveMember(ctx context.Context, userID, serverID uuid.UUID) error {
	_, err := r.DB.ExecContext(ctx,
		`DELETE FROM server_members WHERE user_id = $1 AND server_id = $2`,
		userID, serverID,
	)
	return err
}

func (r *ServerMemberRepo) ListMembers(ctx context.Context, serverID uuid.UUID) ([]models.ServerMember, error) {
	rows, err := r.DB.QueryContext(ctx,
		`SELECT sm.user_id, sm.server_id, sm.nickname, sm.joined_at, u.username, u.display_name, u.avatar_path
		 FROM server_members sm
		 JOIN users u ON u.id = sm.user_id
		 WHERE sm.server_id = $1
		 ORDER BY sm.joined_at`, serverID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var members []models.ServerMember
	for rows.Next() {
		var m models.ServerMember
		if err := rows.Scan(&m.UserID, &m.ServerID, &m.Nickname, &m.JoinedAt, &m.Username, &m.DisplayName, &m.AvatarURL); err != nil {
			return nil, err
		}
		members = append(members, m)
	}
	return members, rows.Err()
}

func (r *ServerMemberRepo) IsMember(ctx context.Context, userID, serverID uuid.UUID) (bool, error) {
	var exists bool
	err := r.DB.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM server_members WHERE user_id = $1 AND server_id = $2)`,
		userID, serverID,
	).Scan(&exists)
	return exists, err
}

// FriendshipRepo handles friendship-related database operations.
type FriendshipRepo struct {
	DB *sql.DB
}

func (r *FriendshipRepo) CreateFriendRequest(ctx context.Context, f *models.Friendship) error {
	f.ID = uuid.New()
	now := time.Now()
	f.CreatedAt = now
	f.UpdatedAt = now
	f.Status = "pending"

	// Enforce user_a < user_b ordering per CHECK constraint
	if f.UserA.String() > f.UserB.String() {
		f.UserA, f.UserB = f.UserB, f.UserA
	}

	_, err := r.DB.ExecContext(ctx,
		`INSERT INTO friendships (id, user_a, user_b, status, initiated_by, dm_channel_id, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		f.ID, f.UserA, f.UserB, f.Status, f.InitiatedBy, f.DMChannelID, f.CreatedAt, f.UpdatedAt,
	)
	return err
}

func (r *FriendshipRepo) AcceptFriend(ctx context.Context, id uuid.UUID) error {
	_, err := r.DB.ExecContext(ctx,
		`UPDATE friendships SET status = 'accepted', updated_at = $1 WHERE id = $2`,
		time.Now(), id,
	)
	return err
}

func (r *FriendshipRepo) GetFriendship(ctx context.Context, userA, userB uuid.UUID) (*models.Friendship, error) {
	// Enforce ordering to match CHECK constraint
	if userA.String() > userB.String() {
		userA, userB = userB, userA
	}

	f := &models.Friendship{}
	err := r.DB.QueryRowContext(ctx,
		`SELECT id, user_a, user_b, status, initiated_by, dm_channel_id, created_at, updated_at
		 FROM friendships WHERE user_a = $1 AND user_b = $2`, userA, userB,
	).Scan(&f.ID, &f.UserA, &f.UserB, &f.Status, &f.InitiatedBy, &f.DMChannelID, &f.CreatedAt, &f.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return f, nil
}

func (r *FriendshipRepo) ListFriends(ctx context.Context, userID uuid.UUID) ([]models.Friendship, error) {
	rows, err := r.DB.QueryContext(ctx,
		`SELECT id, user_a, user_b, status, initiated_by, dm_channel_id, created_at, updated_at
		 FROM friendships
		 WHERE (user_a = $1 OR user_b = $1) AND status = 'accepted'
		 ORDER BY updated_at DESC`, userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var friends []models.Friendship
	for rows.Next() {
		var f models.Friendship
		if err := rows.Scan(&f.ID, &f.UserA, &f.UserB, &f.Status, &f.InitiatedBy, &f.DMChannelID, &f.CreatedAt, &f.UpdatedAt); err != nil {
			return nil, err
		}
		friends = append(friends, f)
	}
	return friends, rows.Err()
}

func (r *FriendshipRepo) GetByID(ctx context.Context, id uuid.UUID) (*models.Friendship, error) {
	f := &models.Friendship{}
	err := r.DB.QueryRowContext(ctx,
		`SELECT id, user_a, user_b, status, initiated_by, dm_channel_id, created_at, updated_at
		 FROM friendships WHERE id = $1`, id,
	).Scan(&f.ID, &f.UserA, &f.UserB, &f.Status, &f.InitiatedBy, &f.DMChannelID, &f.CreatedAt, &f.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return f, nil
}

func (r *FriendshipRepo) SetDMChannelID(ctx context.Context, id, channelID uuid.UUID) error {
	_, err := r.DB.ExecContext(ctx,
		`UPDATE friendships SET dm_channel_id = $1, updated_at = $2 WHERE id = $3`,
		channelID, time.Now(), id,
	)
	return err
}

// UserRepo helper: look up a user by username.
func (r *UserRepo) GetByUsername(ctx context.Context, username string) (*models.User, error) {
	u := &models.User{}
	err := r.DB.QueryRowContext(ctx,
		`SELECT id, username, display_name, avatar_path, tailscale_id, password_hash, status, created_at, updated_at
		 FROM users WHERE username = $1`, username,
	).Scan(&u.ID, &u.Username, &u.DisplayName, &u.AvatarPath, &u.TailscaleID, &u.PasswordHash, &u.Status, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return u, nil
}

// ServerRepo helper: look up a server by invite code.
func (r *ServerRepo) GetServerByInviteCode(ctx context.Context, code string) (*models.Server, error) {
	s := &models.Server{}
	err := r.DB.QueryRowContext(ctx,
		`SELECT id, name, icon_path, owner_id, invite_code, created_at
		 FROM servers WHERE invite_code = $1`, code,
	).Scan(&s.ID, &s.Name, &s.IconPath, &s.OwnerID, &s.InviteCode, &s.CreatedAt)
	if err != nil {
		return nil, err
	}
	return s, nil
}

// DMMemberRepo handles DM channel membership.
type DMMemberRepo struct {
	DB *sql.DB
}

func (r *DMMemberRepo) AddMember(ctx context.Context, channelID, userID uuid.UUID) error {
	_, err := r.DB.ExecContext(ctx,
		`INSERT INTO dm_members (channel_id, user_id, joined_at) VALUES ($1, $2, $3)`,
		channelID, userID, time.Now(),
	)
	return err
}

func (r *DMMemberRepo) IsMember(ctx context.Context, channelID, userID uuid.UUID) (bool, error) {
	var exists bool
	err := r.DB.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM dm_members WHERE channel_id = $1 AND user_id = $2)`,
		channelID, userID,
	).Scan(&exists)
	return exists, err
}

// MessageRepo handles message-related database operations.
type MessageRepo struct {
	DB *sql.DB
}

func (r *MessageRepo) Create(ctx context.Context, m *models.Message) error {
	m.ID = uuid.New()
	now := time.Now()
	m.CreatedAt = now
	m.UpdatedAt = now
	err := r.DB.QueryRowContext(ctx,
		`WITH ins AS (
			INSERT INTO messages (id, channel_id, author_id, content, edited, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			RETURNING author_id
		)
		SELECT u.username, u.display_name, u.avatar_path FROM ins JOIN users u ON u.id = ins.author_id`,
		m.ID, m.ChannelID, m.AuthorID, m.Content, m.Edited, m.CreatedAt, m.UpdatedAt,
	).Scan(&m.AuthorUsername, &m.AuthorDisplayName, &m.AuthorAvatarURL)
	return err
}

func (r *MessageRepo) GetByID(ctx context.Context, id uuid.UUID) (*models.Message, error) {
	m := &models.Message{}
	err := r.DB.QueryRowContext(ctx,
		`SELECT m.id, m.channel_id, m.author_id, m.content, m.edited, m.created_at, m.updated_at, u.username, u.display_name, u.avatar_path
		 FROM messages m
		 JOIN users u ON u.id = m.author_id
		 WHERE m.id = $1`, id,
	).Scan(&m.ID, &m.ChannelID, &m.AuthorID, &m.Content, &m.Edited, &m.CreatedAt, &m.UpdatedAt, &m.AuthorUsername, &m.AuthorDisplayName, &m.AuthorAvatarURL)
	if err != nil {
		return nil, err
	}
	return m, nil
}

func (r *MessageRepo) Update(ctx context.Context, messageID, authorID uuid.UUID, content string) (*models.Message, error) {
	m := &models.Message{}
	err := r.DB.QueryRowContext(ctx,
		`WITH upd AS (
			UPDATE messages SET content = $1, edited = true, updated_at = $2
			WHERE id = $3 AND author_id = $4
			RETURNING id, channel_id, author_id, content, edited, created_at, updated_at
		)
		SELECT upd.id, upd.channel_id, upd.author_id, upd.content, upd.edited, upd.created_at, upd.updated_at, u.username, u.display_name, u.avatar_path
		FROM upd JOIN users u ON u.id = upd.author_id`,
		content, time.Now(), messageID, authorID,
	).Scan(&m.ID, &m.ChannelID, &m.AuthorID, &m.Content, &m.Edited, &m.CreatedAt, &m.UpdatedAt, &m.AuthorUsername, &m.AuthorDisplayName, &m.AuthorAvatarURL)
	if err != nil {
		return nil, err
	}
	return m, nil
}

func (r *MessageRepo) Delete(ctx context.Context, messageID, authorID uuid.UUID) error {
	result, err := r.DB.ExecContext(ctx,
		`DELETE FROM messages WHERE id = $1 AND author_id = $2`,
		messageID, authorID,
	)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *MessageRepo) ListByChannel(ctx context.Context, channelID uuid.UUID, before *uuid.UUID, limit int) ([]models.Message, error) {
	var rows *sql.Rows
	var err error

	if before != nil {
		rows, err = r.DB.QueryContext(ctx,
			`SELECT m.id, m.channel_id, m.author_id, m.content, m.edited, m.created_at, m.updated_at, u.username, u.display_name, u.avatar_path
			 FROM messages m
			 JOIN users u ON u.id = m.author_id
			 WHERE m.channel_id = $1
			   AND m.created_at < (SELECT created_at FROM messages WHERE id = $2)
			 ORDER BY m.created_at DESC
			 LIMIT $3`, channelID, *before, limit,
		)
	} else {
		rows, err = r.DB.QueryContext(ctx,
			`SELECT m.id, m.channel_id, m.author_id, m.content, m.edited, m.created_at, m.updated_at, u.username, u.display_name, u.avatar_path
			 FROM messages m
			 JOIN users u ON u.id = m.author_id
			 WHERE m.channel_id = $1
			 ORDER BY m.created_at DESC
			 LIMIT $2`, channelID, limit,
		)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []models.Message
	for rows.Next() {
		var m models.Message
		if err := rows.Scan(&m.ID, &m.ChannelID, &m.AuthorID, &m.Content, &m.Edited, &m.CreatedAt, &m.UpdatedAt, &m.AuthorUsername, &m.AuthorDisplayName, &m.AuthorAvatarURL); err != nil {
			return nil, err
		}
		messages = append(messages, m)
	}
	return messages, rows.Err()
}
