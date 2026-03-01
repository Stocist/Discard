-- 002_channel_read_state.sql
-- Track per-user read position in each channel for unread indicators.

CREATE TABLE channel_read_state (
    user_id             UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    channel_id          UUID NOT NULL REFERENCES channels(id) ON DELETE CASCADE,
    last_read_message_id UUID,
    last_read_at        TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (user_id, channel_id)
);
