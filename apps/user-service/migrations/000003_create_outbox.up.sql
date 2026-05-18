SET search_path TO users;

CREATE TABLE IF NOT EXISTS outbox (
    id         BIGSERIAL    PRIMARY KEY,
    topic      TEXT         NOT NULL,
    key        TEXT         NOT NULL,
    payload    JSONB        NOT NULL,
    trace_id   TEXT         NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    sent_at    TIMESTAMPTZ
);

-- partial index: only unsent rows; keeps index tiny as sent rows accumulate
CREATE INDEX IF NOT EXISTS idx_outbox_unsent ON outbox(id) WHERE sent_at IS NULL;
