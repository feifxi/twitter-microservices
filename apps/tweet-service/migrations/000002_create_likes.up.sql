SET search_path TO tweet;

CREATE TABLE IF NOT EXISTS likes (
    user_id    TEXT        NOT NULL,
    tweet_id   TEXT        NOT NULL REFERENCES tweets(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, tweet_id)
);

CREATE INDEX IF NOT EXISTS idx_likes_tweet_id ON likes(tweet_id);
CREATE INDEX IF NOT EXISTS idx_likes_user_created ON likes(user_id, created_at DESC, tweet_id DESC);
