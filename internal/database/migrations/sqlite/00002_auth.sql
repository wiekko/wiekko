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
    id            TEXT PRIMARY KEY,
    username      TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    is_admin      INTEGER NOT NULL DEFAULT 0,
    status        TEXT NOT NULL DEFAULT 'active',
    totp_secret   TEXT NOT NULL DEFAULT '',
    totp_enabled  INTEGER NOT NULL DEFAULT 0,
    created_at    TEXT NOT NULL,
    updated_at    TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS sessions (
    id         TEXT PRIMARY KEY,
    user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    csrf_token TEXT NOT NULL,
    created_at TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    ip_address TEXT NOT NULL DEFAULT '',
    user_agent TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_sessions_expires_at ON sessions(expires_at);

CREATE TABLE IF NOT EXISTS personal_access_tokens (
    id           TEXT PRIMARY KEY,
    user_id      TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name         TEXT NOT NULL,
    token_hash   TEXT NOT NULL UNIQUE,
    token_prefix TEXT NOT NULL,
    last_used_at TEXT,
    expires_at   TEXT,
    created_at   TEXT NOT NULL,
    revoked_at   TEXT
);
CREATE INDEX IF NOT EXISTS idx_pat_user_id ON personal_access_tokens(user_id);
CREATE INDEX IF NOT EXISTS idx_pat_token_hash ON personal_access_tokens(token_hash);

CREATE TABLE IF NOT EXISTS invitations (
    id           TEXT PRIMARY KEY,
    token_hash   TEXT NOT NULL UNIQUE,
    invited_by   TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    product_id   TEXT,
    role         TEXT,
    accepted_at  TEXT,
    expires_at   TEXT NOT NULL,
    created_at   TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_invitations_token_hash ON invitations(token_hash);

CREATE TABLE IF NOT EXISTS personal_data (
    id          TEXT PRIMARY KEY,
    owner_table TEXT NOT NULL,
    owner_id    TEXT NOT NULL,
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
DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS settings;
