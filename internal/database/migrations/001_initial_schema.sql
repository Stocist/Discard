-- 001_initial_schema.sql
-- Phase 1 tables for Discard

CREATE TABLE users (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    username        VARCHAR(32) UNIQUE NOT NULL,
    display_name    VARCHAR(64),
    avatar_path     VARCHAR(512),
    tailscale_id    VARCHAR(256) UNIQUE,
    password_hash   VARCHAR(256),
    status          VARCHAR(16) DEFAULT 'offline',
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    updated_at      TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE servers (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name            VARCHAR(100) NOT NULL,
    icon_path       VARCHAR(512),
    owner_id        UUID NOT NULL REFERENCES users(id),
    invite_code     VARCHAR(16) UNIQUE,
    created_at      TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE channels (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    server_id       UUID REFERENCES servers(id) ON DELETE CASCADE,
    name            VARCHAR(100),
    topic           TEXT,
    type            VARCHAR(16) NOT NULL DEFAULT 'text',
    position        INTEGER DEFAULT 0,
    created_at      TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE messages (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    channel_id      UUID NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    author_id       UUID NOT NULL REFERENCES users(id),
    content         TEXT NOT NULL,
    edited          BOOLEAN DEFAULT FALSE,
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    updated_at      TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_messages_channel_created ON messages(channel_id, created_at DESC);
CREATE INDEX idx_messages_content_fts ON messages USING GIN(to_tsvector('english', content));

CREATE TABLE attachments (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    message_id      UUID NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    file_path       VARCHAR(512) NOT NULL,
    original_name   VARCHAR(256) NOT NULL,
    mime_type       VARCHAR(128),
    file_size       BIGINT,
    width           INTEGER,
    height          INTEGER,
    created_at      TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE server_members (
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    server_id       UUID NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    nickname        VARCHAR(64),
    joined_at       TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (user_id, server_id)
);

CREATE TABLE roles (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    server_id       UUID NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    name            VARCHAR(64) NOT NULL,
    color           VARCHAR(7),
    permissions     BIGINT DEFAULT 0,
    position        INTEGER DEFAULT 0,
    created_at      TIMESTAMPTZ DEFAULT NOW()
);

CREATE TABLE member_roles (
    user_id         UUID NOT NULL,
    server_id       UUID NOT NULL,
    role_id         UUID NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    PRIMARY KEY (user_id, server_id, role_id),
    FOREIGN KEY (user_id, server_id) REFERENCES server_members(user_id, server_id) ON DELETE CASCADE
);

CREATE TABLE dm_members (
    channel_id      UUID NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    user_id         UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    joined_at       TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (channel_id, user_id)
);

CREATE TABLE friendships (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_a          UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    user_b          UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    status          VARCHAR(16) NOT NULL DEFAULT 'pending',
    initiated_by    UUID NOT NULL REFERENCES users(id),
    dm_channel_id   UUID REFERENCES channels(id),
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    updated_at      TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE(user_a, user_b),
    CHECK (user_a < user_b)
);
