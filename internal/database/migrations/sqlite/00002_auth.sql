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
INSERT INTO settings (key, value) VALUES ('pat_max_lifetime_days', '0');

CREATE TABLE IF NOT EXISTS users (
    id            TEXT PRIMARY KEY,
    username      TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    is_admin      INTEGER NOT NULL DEFAULT 0,
    status        TEXT NOT NULL DEFAULT 'active',
    totp_secret   TEXT NOT NULL DEFAULT '',
    totp_enabled  INTEGER NOT NULL DEFAULT 0,
    totp_last_step INTEGER NOT NULL DEFAULT 0,
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

-- TOTP recovery codes (SHA-256 hashes; each usable once).
CREATE TABLE IF NOT EXISTS totp_recovery_codes (
    id         TEXT PRIMARY KEY,
    user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    code_hash  TEXT NOT NULL,
    created_at TEXT NOT NULL,
    used_at    TEXT
);
CREATE INDEX IF NOT EXISTS idx_totp_recovery_codes_user_id ON totp_recovery_codes(user_id);

-- Pending second login step: the password was accepted, the TOTP code is
-- still outstanding. The id is the SHA-256 of the token handed to the client.
CREATE TABLE IF NOT EXISTS login_challenges (
    id         TEXT PRIMARY KEY,
    user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    attempts   INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    ip_address TEXT NOT NULL DEFAULT '',
    user_agent TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_login_challenges_user_id ON login_challenges(user_id);

CREATE TABLE IF NOT EXISTS personal_access_tokens (
    id           TEXT PRIMARY KEY,
    user_id      TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name         TEXT NOT NULL,
    token_hash   TEXT NOT NULL UNIQUE,
    token_prefix TEXT NOT NULL,
    last_used_at TEXT,
    expires_at   TEXT,
    created_at   TEXT NOT NULL,
    revoked_at   TEXT,
    -- Restrictions (SC-401 – SC-406): space-separated "scope:level" grants,
    -- an optional role cap, and whether only the products listed in
    -- personal_access_token_products (00003) are reachable.
    scopes       TEXT NOT NULL,
    role_cap     TEXT,
    products_restricted INTEGER NOT NULL DEFAULT 0
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

-- Password reset links (SC-110). Like sessions and invitations, the table
-- stores only the SHA-256 of the token sent by email, so a copy of the
-- database holds nothing that resets a password. A token works once
-- (used_at) and until expires_at.
CREATE TABLE IF NOT EXISTS password_reset_tokens (
    id         TEXT PRIMARY KEY,
    user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,
    created_at TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    used_at    TEXT
);
CREATE INDEX IF NOT EXISTS idx_password_reset_tokens_user_id ON password_reset_tokens(user_id);

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
