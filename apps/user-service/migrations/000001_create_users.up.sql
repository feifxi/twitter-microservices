CREATE SCHEMA IF NOT EXISTS users;
SET search_path TO users;

CREATE TABLE IF NOT EXISTS users (
    id               TEXT        PRIMARY KEY,        -- Keycloak sub UUID (stable, provider-issued)
    email            TEXT        UNIQUE NOT NULL,
    username         TEXT        UNIQUE,             -- NULL until onboarding is complete
    display_name     TEXT,
    avatar_url       TEXT,
    header_image_url TEXT,
    bio              TEXT,
    website_url      TEXT,
    location         TEXT,
    follower_count   INTEGER     NOT NULL DEFAULT 0,
    following_count  INTEGER     NOT NULL DEFAULT 0,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);
