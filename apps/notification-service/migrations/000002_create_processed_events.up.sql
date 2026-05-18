SET search_path TO notification;

-- Deduplication table for at-least-once consumer idempotency.
-- event_id is the outbox row ID carried in the X-Event-ID Kafka header.
-- Rows older than 7 days are safe to prune (no broker will redeliver that old).
CREATE TABLE IF NOT EXISTS processed_events (
    event_id     BIGINT      PRIMARY KEY,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
