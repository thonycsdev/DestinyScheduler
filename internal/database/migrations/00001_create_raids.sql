-- +goose Up
CREATE TABLE raids (
    id         uuid PRIMARY KEY DEFAULT uuidv7(),
    name       text NOT NULL,
    starts_at  timestamptz NOT NULL,
    leader_id  text NOT NULL,         -- Discord user ID
    channel_id text,                  -- set once the raid message is posted
    message_id text
);

CREATE INDEX raids_starts_at_idx ON raids (starts_at);

CREATE TABLE raid_members (
    raid_id   uuid NOT NULL REFERENCES raids (id) ON DELETE CASCADE,
    user_id   text NOT NULL,          -- Discord user ID
    joined_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY (raid_id, user_id)
);

-- +goose Down
DROP TABLE raid_members;
DROP TABLE raids;
