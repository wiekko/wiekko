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

CREATE TABLE IF NOT EXISTS users (
    id            TEXT PRIMARY KEY,
    username      TEXT NOT NULL UNIQUE,
    email         TEXT NOT NULL UNIQUE,
    display_name  TEXT NOT NULL DEFAULT '',
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

CREATE TABLE IF NOT EXISTS product_roles (
    id         TEXT PRIMARY KEY,
    user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    product_id TEXT NOT NULL,
    role       TEXT NOT NULL,
    created_at TEXT NOT NULL,
    UNIQUE(user_id, product_id)
);
CREATE INDEX IF NOT EXISTS idx_product_roles_user_id ON product_roles(user_id);
CREATE INDEX IF NOT EXISTS idx_product_roles_product_id ON product_roles(product_id);

CREATE TABLE IF NOT EXISTS invitations (
    id           TEXT PRIMARY KEY,
    email        TEXT NOT NULL,
    token_hash   TEXT NOT NULL UNIQUE,
    invited_by   TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    product_id   TEXT,
    role         TEXT,
    accepted_at  TEXT,
    expires_at   TEXT NOT NULL,
    created_at   TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_invitations_token_hash ON invitations(token_hash);
CREATE INDEX IF NOT EXISTS idx_invitations_email ON invitations(email);

-- +goose Down
DROP TABLE IF EXISTS invitations;
DROP TABLE IF EXISTS product_roles;
DROP TABLE IF EXISTS personal_access_tokens;
DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS settings;
