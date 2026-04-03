package store

import (
	"database/sql"

	"nexmap.net/internal/crypto"
)

// Store groups all data access interfaces.
type Store struct {
	User       UserStore
	Session    SessionStore
	PAT        PATStore
	Setting    SettingStore
	Invitation InvitationStore
}

// New creates a Store backed by the given database connection.
func New(db *sql.DB, keys *crypto.KeyRing) *Store {
	return &Store{
		User:       &userStore{db: db, keys: keys},
		Session:    &sessionStore{db: db},
		PAT:        &patStore{db: db},
		Setting:    &settingStore{db: db},
		Invitation: &invitationStore{db: db, keys: keys},
	}
}
