CREATE SCHEMA IF NOT EXISTS tweet;
SET search_path TO tweet;

CREATE TABLE IF NOT EXISTS tweets (
    id                   TEXT        PRIMARY KEY,
    author_id            TEXT        NOT NULL,
    body                 TEXT        NOT NULL CHECK (char_length(body) <= 280),
    reply_to_id          TEXT        REFERENCES tweets(id) ON DELETE SET NULL,
    media_id             TEXT,
    media_url            TEXT,
    like_count           INTEGER     NOT NULL DEFAULT 0,
    retweet_count        INTEGER     NOT NULL DEFAULT 0,
    reply_count          INTEGER     NOT NULL DEFAULT 0,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_tweets_author_id        ON tweets(author_id);
CREATE INDEX IF NOT EXISTS idx_tweets_created_at       ON tweets(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_tweets_reply_to_id      ON tweets(reply_to_id, created_at ASC);
CREATE INDEX IF NOT EXISTS idx_tweets_author_media     ON tweets(author_id, created_at DESC) WHERE media_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS retweets (
    id                TEXT        PRIMARY KEY,
    retweeter_id      TEXT        NOT NULL,
    original_tweet_id TEXT        NOT NULL REFERENCES tweets(id) ON DELETE CASCADE,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (retweeter_id, original_tweet_id)
);

CREATE INDEX IF NOT EXISTS idx_retweets_retweeter_created ON retweets(retweeter_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_retweets_original          ON retweets(original_tweet_id);
