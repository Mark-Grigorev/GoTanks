CREATE TABLE IF NOT EXISTS users (
    id          BIGSERIAL PRIMARY KEY,
    telegram_id BIGINT    NOT NULL UNIQUE,
    username    TEXT      NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS user_stats (
    user_id BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    wins    INT NOT NULL DEFAULT 0,
    losses  INT NOT NULL DEFAULT 0,
    kills   INT NOT NULL DEFAULT 0,
    deaths  INT NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS matches (
    id          BIGSERIAL PRIMARY KEY,
    map_id      TEXT        NOT NULL,
    started_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS match_players (
    match_id  BIGINT REFERENCES matches(id) ON DELETE CASCADE,
    user_id   BIGINT REFERENCES users(id)   ON DELETE CASCADE,
    tank_type TEXT   NOT NULL,
    result    TEXT   NOT NULL,
    PRIMARY KEY (match_id, user_id)
);
