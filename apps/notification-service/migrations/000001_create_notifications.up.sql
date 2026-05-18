CREATE SCHEMA IF NOT EXISTS notification;
SET search_path TO notification;

CREATE TYPE notif_type AS ENUM ('like', 'retweet', 'reply', 'follow');

CREATE TABLE IF NOT EXISTS notifications (
    id         TEXT        PRIMARY KEY,
    user_id    TEXT        NOT NULL,
    actor_id   TEXT        NOT NULL,
    type       notif_type  NOT NULL,
    tweet_id   TEXT,
    read_at    TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_notifications_user_id ON notifications(user_id, created_at DESC);
