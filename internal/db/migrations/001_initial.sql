-- 001_initial.sql: Initial schema for penpal relay server

CREATE EXTENSION IF NOT EXISTS "pgcrypto";

CREATE TABLE users (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    username        TEXT NOT NULL,
    discriminator   CHAR(4) NOT NULL,
    public_key      BYTEA NOT NULL,
    home_city       TEXT NOT NULL,
    home_lat        DOUBLE PRECISION NOT NULL,
    home_lng        DOUBLE PRECISION NOT NULL,
    last_active     TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(username, discriminator)
);

CREATE INDEX idx_users_username ON users(username);
CREATE INDEX idx_users_last_active ON users(last_active);

CREATE TABLE contacts (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    contact_id  UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE(owner_id, contact_id)
);

CREATE INDEX idx_contacts_owner ON contacts(owner_id);

CREATE TABLE messages (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    sender_id       UUID NOT NULL REFERENCES users(id),
    recipient_id    UUID NOT NULL REFERENCES users(id),
    encrypted_body  BYTEA NOT NULL,
    shipping_tier   TEXT NOT NULL CHECK (shipping_tier IN ('first_class', 'priority', 'express')),
    route           JSONB NOT NULL,
    sent_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    release_at      TIMESTAMPTZ NOT NULL,
    delivered_at    TIMESTAMPTZ,
    read_at         TIMESTAMPTZ,
    status          TEXT NOT NULL DEFAULT 'in_transit'
                    CHECK (status IN ('in_transit', 'delivered', 'read'))
);

CREATE INDEX idx_messages_recipient_status ON messages(recipient_id, status);
CREATE INDEX idx_messages_sender ON messages(sender_id);
CREATE INDEX idx_messages_release ON messages(status, release_at) WHERE status = 'in_transit';

CREATE TABLE stamps (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    stamp_type  TEXT NOT NULL,
    rarity      TEXT NOT NULL CHECK (rarity IN ('common', 'rare', 'ultra_rare')),
    earned_via  TEXT NOT NULL CHECK (earned_via IN ('registration', 'delivery', 'route', 'transfer', 'weekly')),
    source_msg  UUID REFERENCES messages(id),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_stamps_owner ON stamps(owner_id);

CREATE TABLE stamp_attachments (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    message_id  UUID NOT NULL REFERENCES messages(id),
    stamp_id    UUID NOT NULL REFERENCES stamps(id),
    UNIQUE(stamp_id)
);

CREATE INDEX idx_stamp_attachments_message ON stamp_attachments(message_id);

CREATE TABLE blocks (
    blocker_id  UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    blocked_id  UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (blocker_id, blocked_id)
);
