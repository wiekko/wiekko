-- +goose Up

CREATE TABLE IF NOT EXISTS settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

INSERT INTO settings (key, value) VALUES ('self_registration_enabled', 'false');
INSERT INTO settings (key, value) VALUES ('moderation_enabled', 'false');
INSERT INTO settings (key, value) VALUES ('2fa_enforced', 'false');
INSERT INTO settings (key, value) VALUES ('password_min_length', '10');
INSERT INTO settings (key, value) VALUES ('password_require_uppercase', 'true');
INSERT INTO settings (key, value) VALUES ('password_require_lowercase', 'true');
INSERT INTO settings (key, value) VALUES ('password_require_digit', 'true');
INSERT INTO settings (key, value) VALUES ('password_require_special', 'false');
INSERT INTO settings (key, value) VALUES ('product_creation_enabled', 'true');

CREATE TABLE IF NOT EXISTS users (
    id            UUID PRIMARY KEY,
    username      TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    is_admin      BOOLEAN NOT NULL DEFAULT FALSE,
    status        TEXT NOT NULL DEFAULT 'active',
    totp_secret   TEXT NOT NULL DEFAULT '',
    totp_enabled  BOOLEAN NOT NULL DEFAULT FALSE,
    totp_last_step BIGINT NOT NULL DEFAULT 0,
    created_at    TIMESTAMPTZ NOT NULL,
    updated_at    TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS sessions (
    id         UUID PRIMARY KEY,
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    csrf_token TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    ip_address TEXT NOT NULL DEFAULT '',
    user_agent TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_sessions_expires_at ON sessions(expires_at);

-- TOTP recovery codes (SHA-256 hashes; each usable once).
CREATE TABLE IF NOT EXISTS totp_recovery_codes (
    id         UUID PRIMARY KEY,
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    code_hash  TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    used_at    TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_totp_recovery_codes_user_id ON totp_recovery_codes(user_id);

-- Pending second login step: the password was accepted, the TOTP code is
-- still outstanding. The id is the SHA-256 of the token handed to the client.
CREATE TABLE IF NOT EXISTS login_challenges (
    id         TEXT PRIMARY KEY,
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    attempts   INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    ip_address TEXT NOT NULL DEFAULT '',
    user_agent TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_login_challenges_user_id ON login_challenges(user_id);

CREATE TABLE IF NOT EXISTS personal_access_tokens (
    id           UUID PRIMARY KEY,
    user_id      UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name         TEXT NOT NULL,
    token_hash   TEXT NOT NULL UNIQUE,
    token_prefix TEXT NOT NULL,
    last_used_at TIMESTAMPTZ,
    expires_at   TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL,
    revoked_at   TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_pat_user_id ON personal_access_tokens(user_id);
CREATE INDEX IF NOT EXISTS idx_pat_token_hash ON personal_access_tokens(token_hash);

CREATE TABLE IF NOT EXISTS invitations (
    id           UUID PRIMARY KEY,
    token_hash   TEXT NOT NULL UNIQUE,
    invited_by   UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    product_id   UUID,
    role         TEXT,
    accepted_at  TIMESTAMPTZ,
    expires_at   TIMESTAMPTZ NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_invitations_token_hash ON invitations(token_hash);

CREATE TABLE IF NOT EXISTS personal_data (
    id          UUID PRIMARY KEY,
    owner_table TEXT NOT NULL,
    owner_id    UUID NOT NULL,
    field_name  TEXT NOT NULL,
    ciphertext  TEXT NOT NULL,
    key_id      TEXT NOT NULL,
    blind_index TEXT,
    UNIQUE(owner_table, owner_id, field_name)
);
CREATE INDEX IF NOT EXISTS idx_pd_owner ON personal_data(owner_table, owner_id);
CREATE INDEX IF NOT EXISTS idx_pd_blind ON personal_data(blind_index) WHERE blind_index IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_pd_unique_user_email
    ON personal_data(blind_index)
    WHERE owner_table = 'users' AND field_name = 'email';

-- +goose Down
DROP TABLE IF EXISTS personal_data;
DROP TABLE IF EXISTS invitations;
DROP TABLE IF EXISTS personal_access_tokens;
DROP TABLE IF EXISTS login_challenges;
DROP TABLE IF EXISTS totp_recovery_codes;
DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS settings;
