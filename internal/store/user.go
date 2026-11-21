// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"wiekko.net/internal/crypto"
	"wiekko.net/internal/model"
)

// UserFilter controls which users are returned by List.
type UserFilter struct {
	Status string // empty = all statuses
}

// UserStore defines data access operations for users.
type UserStore interface {
	Create(ctx context.Context, user *model.User) error
	GetByID(ctx context.Context, id string) (*model.User, error)
	GetByUsername(ctx context.Context, username string) (*model.User, error)
	GetByEmail(ctx context.Context, email string) (*model.User, error)
	Update(ctx context.Context, user *model.User) error
	UpdateStatus(ctx context.Context, id, status string) error
	UpdatePassword(ctx context.Context, id, hash string) error
	Delete(ctx context.Context, id string) error
	List(ctx context.Context, filter UserFilter) ([]*model.User, error)
	// SetTOTP stores the user's TOTP secret (encrypted) and enabled flag and
	// resets the replay guard. An empty secret removes TOTP entirely.
	SetTOTP(ctx context.Context, id, secret string, enabled bool) error
	// ConsumeTOTPStep records step as used. It returns false when this step
	// or a later one was already used, i.e. the code is a replay.
	ConsumeTOTPStep(ctx context.Context, id string, step int64) (bool, error)
	Count(ctx context.Context) (int64, error)
}

type userStore struct {
	db   *DB
	keys *crypto.KeyRing
}

const userSelectColumns = `u.id, u.username, u.password_hash, u.is_admin, u.status,
	u.totp_secret, u.totp_enabled, u.created_at, u.updated_at,
	pd_email.ciphertext, pd_email.key_id,
	pd_dn.ciphertext, pd_dn.key_id`

const userJoins = `FROM users u
	LEFT JOIN personal_data pd_email ON pd_email.owner_table = 'users' AND pd_email.owner_id = u.id AND pd_email.field_name = 'email'
	LEFT JOIN personal_data pd_dn ON pd_dn.owner_table = 'users' AND pd_dn.owner_id = u.id AND pd_dn.field_name = 'display_name'`

func (s *userStore) Create(ctx context.Context, user *model.User) error {
	totpSecret, err := s.encryptTOTPSecret(user.ID, user.TOTPSecret)
	if err != nil {
		return fmt.Errorf("user create: %w", err)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("user create: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	_, err = tx.ExecContext(ctx,
		`INSERT INTO users (id, username, password_hash, is_admin, status, totp_secret, totp_enabled, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		user.ID, user.Username, user.PasswordHash,
		user.IsAdmin, user.Status, totpSecret, user.TOTPEnabled,
		user.CreatedAt, user.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("user create: insert user: %w", err)
	}

	if err := s.insertPersonalData(ctx, tx, user); err != nil {
		return fmt.Errorf("user create: %w", err)
	}

	return tx.Commit()
}

func (s *userStore) GetByID(ctx context.Context, id string) (*model.User, error) {
	return s.getOne(ctx, `SELECT `+userSelectColumns+` `+userJoins+` WHERE u.id = ?`, id)
}

func (s *userStore) GetByUsername(ctx context.Context, username string) (*model.User, error) {
	// Usernames are unique regardless of case: "Admin" must not be registrable
	// next to "admin". LOWER runs in the database on both sides, so the two
	// agree on what lowercase means whichever engine is used.
	return s.getOne(ctx, `SELECT `+userSelectColumns+` `+userJoins+` WHERE LOWER(u.username) = LOWER(?)`,
		strings.TrimSpace(username))
}

func (s *userStore) GetByEmail(ctx context.Context, email string) (*model.User, error) {
	blind := s.emailIndex(email)
	return s.getOne(ctx,
		`SELECT `+userSelectColumns+` `+userJoins+`
		 JOIN personal_data pd_lookup ON pd_lookup.owner_table = 'users' AND pd_lookup.owner_id = u.id AND pd_lookup.field_name = 'email'
		 WHERE pd_lookup.blind_index = ?`, blind)
}

func (s *userStore) Update(ctx context.Context, user *model.User) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("user update: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	_, err = tx.ExecContext(ctx,
		`UPDATE users SET username = ?, is_admin = ?, status = ?, updated_at = ? WHERE id = ?`,
		user.Username, user.IsAdmin, user.Status, user.UpdatedAt, user.ID,
	)
	if err != nil {
		return fmt.Errorf("user update: update user: %w", err)
	}

	if err := s.updatePersonalData(ctx, tx, user); err != nil {
		return fmt.Errorf("user update: %w", err)
	}

	return tx.Commit()
}

func (s *userStore) UpdateStatus(ctx context.Context, id, status string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE users SET status = ?, updated_at = ? WHERE id = ?`, status, Now(), id)
	return err
}

func (s *userStore) UpdatePassword(ctx context.Context, id, hash string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE users SET password_hash = ?, updated_at = ? WHERE id = ?`, hash, Now(), id)
	return err
}

func (s *userStore) Delete(ctx context.Context, id string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("user delete: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	_, err = tx.ExecContext(ctx, `DELETE FROM personal_data WHERE owner_table = 'users' AND owner_id = ?`, id)
	if err != nil {
		return fmt.Errorf("user delete: delete personal data: %w", err)
	}

	_, err = tx.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("user delete: delete user: %w", err)
	}

	return tx.Commit()
}

func (s *userStore) List(ctx context.Context, filter UserFilter) ([]*model.User, error) {
	query := `SELECT ` + userSelectColumns + ` ` + userJoins
	var args []any
	if filter.Status != "" {
		query += ` WHERE u.status = ?`
		args = append(args, filter.Status)
	}
	query += ` ORDER BY u.username`

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)

	var users []*model.User
	for rows.Next() {
		u, err := s.scanUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

func (s *userStore) SetTOTP(ctx context.Context, id, secret string, enabled bool) error {
	stored, err := s.encryptTOTPSecret(id, secret)
	if err != nil {
		return fmt.Errorf("user set totp: %w", err)
	}
	if secret == "" {
		enabled = false
	}
	_, err = s.db.ExecContext(ctx,
		`UPDATE users SET totp_secret = ?, totp_enabled = ?, totp_last_step = 0, updated_at = ? WHERE id = ?`,
		stored, enabled, Now(), id,
	)
	if err != nil {
		return fmt.Errorf("user set totp: %w", err)
	}
	return nil
}

func (s *userStore) ConsumeTOTPStep(ctx context.Context, id string, step int64) (bool, error) {
	// A single conditional UPDATE makes check-and-set atomic, so two requests
	// racing with the same code cannot both succeed.
	res, err := s.db.ExecContext(ctx,
		`UPDATE users SET totp_last_step = ? WHERE id = ? AND totp_last_step < ?`, step, id, step)
	if err != nil {
		return false, fmt.Errorf("user consume totp step: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("user consume totp step: %w", err)
	}
	return n == 1, nil
}

// encryptTOTPSecret encrypts a TOTP secret for the totp_secret column as
// "<key id>:<ciphertext>". Base64 ciphertext never contains a colon, so the
// last colon separates the two even if a key id contains one.
func (s *userStore) encryptTOTPSecret(userID, secret string) (string, error) {
	if secret == "" {
		return "", nil
	}
	ct, keyID, err := s.keys.EncryptFor(secret, personalDataContext("users", userID, "totp_secret"))
	if err != nil {
		return "", fmt.Errorf("encrypt totp secret: %w", err)
	}
	return keyID + ":" + ct, nil
}

func (s *userStore) decryptTOTPSecret(userID, stored string) (string, error) {
	if stored == "" {
		return "", nil
	}
	i := strings.LastIndex(stored, ":")
	if i < 0 {
		return "", errors.New("decrypt totp secret: malformed value")
	}
	// The key ID ends at the first colon that is followed by a ciphertext:
	// neither base64 nor the "v2." prefix of a bound one contains a colon.
	secret, err := s.keys.DecryptFor(stored[i+1:], stored[:i], personalDataContext("users", userID, "totp_secret"))
	if err != nil {
		return "", fmt.Errorf("decrypt totp secret: %w", err)
	}
	return secret, nil
}

// emailIndex is the lookup hash of an email address. Addresses are compared
// without regard to case or surrounding space, so "Ada@Example.com" cannot be
// registered next to "ada@example.com".
func (s *userStore) emailIndex(email string) string {
	return s.keys.BlindIndex(strings.ToLower(strings.TrimSpace(email)))
}

// personalDataContext names the place an encrypted value belongs to. It is
// bound into the ciphertext (crypto.KeyRing.EncryptFor), so a value copied to
// another record or field does not decrypt there.
func personalDataContext(table, ownerID, field string) string {
	return table + ":" + ownerID + ":" + field
}

func (s *userStore) Count(ctx context.Context) (int64, error) {
	var count int64
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&count)
	return count, err
}

func (s *userStore) getOne(ctx context.Context, query string, args ...any) (*model.User, error) {
	row := s.db.QueryRowContext(ctx, query, args...)
	u, err := s.scanUser(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("user not found: %w", err)
	}
	return u, err
}

func (s *userStore) scanUser(sc scanner) (*model.User, error) {
	var u model.User
	var emailCT, emailKeyID, dnCT, dnKeyID sql.NullString

	err := sc.Scan(&u.ID, &u.Username, &u.PasswordHash,
		&u.IsAdmin, &u.Status, &u.TOTPSecret, &u.TOTPEnabled, &u.CreatedAt, &u.UpdatedAt,
		&emailCT, &emailKeyID, &dnCT, &dnKeyID)
	if err != nil {
		return nil, err
	}

	u.TOTPSecret, err = s.decryptTOTPSecret(u.ID, u.TOTPSecret)
	if err != nil {
		return nil, err
	}

	if emailCT.Valid && emailKeyID.Valid {
		u.Email, err = s.keys.DecryptFor(emailCT.String, emailKeyID.String, personalDataContext("users", u.ID, "email"))
		if err != nil {
			return nil, fmt.Errorf("decrypt email: %w", err)
		}
	}

	if dnCT.Valid && dnKeyID.Valid {
		u.DisplayName, err = s.keys.DecryptFor(dnCT.String, dnKeyID.String, personalDataContext("users", u.ID, "display_name"))
		if err != nil {
			return nil, fmt.Errorf("decrypt display_name: %w", err)
		}
	}

	return &u, nil
}

func (s *userStore) insertPersonalData(ctx context.Context, tx *Tx, user *model.User) error {
	emailCT, emailKeyID, err := s.keys.EncryptFor(user.Email, personalDataContext("users", user.ID, "email"))
	if err != nil {
		return fmt.Errorf("encrypt email: %w", err)
	}
	emailBlind := s.emailIndex(user.Email)

	_, err = tx.ExecContext(ctx,
		`INSERT INTO personal_data (id, owner_table, owner_id, field_name, ciphertext, key_id, blind_index)
		 VALUES (?, 'users', ?, 'email', ?, ?, ?)`,
		uuid.New().String(), user.ID, emailCT, emailKeyID, emailBlind,
	)
	if err != nil {
		return fmt.Errorf("insert email pd: %w", err)
	}

	dnCT, dnKeyID, err := s.keys.EncryptFor(user.DisplayName, personalDataContext("users", user.ID, "display_name"))
	if err != nil {
		return fmt.Errorf("encrypt display_name: %w", err)
	}

	_, err = tx.ExecContext(ctx,
		`INSERT INTO personal_data (id, owner_table, owner_id, field_name, ciphertext, key_id, blind_index)
		 VALUES (?, 'users', ?, 'display_name', ?, ?, NULL)`,
		uuid.New().String(), user.ID, dnCT, dnKeyID,
	)
	if err != nil {
		return fmt.Errorf("insert display_name pd: %w", err)
	}

	return nil
}

func (s *userStore) updatePersonalData(ctx context.Context, tx *Tx, user *model.User) error {
	emailCT, emailKeyID, err := s.keys.EncryptFor(user.Email, personalDataContext("users", user.ID, "email"))
	if err != nil {
		return fmt.Errorf("encrypt email: %w", err)
	}
	emailBlind := s.emailIndex(user.Email)

	_, err = tx.ExecContext(ctx,
		`UPDATE personal_data SET ciphertext = ?, key_id = ?, blind_index = ?
		 WHERE owner_table = 'users' AND owner_id = ? AND field_name = 'email'`,
		emailCT, emailKeyID, emailBlind, user.ID,
	)
	if err != nil {
		return fmt.Errorf("update email pd: %w", err)
	}

	dnCT, dnKeyID, err := s.keys.EncryptFor(user.DisplayName, personalDataContext("users", user.ID, "display_name"))
	if err != nil {
		return fmt.Errorf("encrypt display_name: %w", err)
	}

	_, err = tx.ExecContext(ctx,
		`UPDATE personal_data SET ciphertext = ?, key_id = ?
		 WHERE owner_table = 'users' AND owner_id = ? AND field_name = 'display_name'`,
		dnCT, dnKeyID, user.ID,
	)
	if err != nil {
		return fmt.Errorf("update display_name pd: %w", err)
	}

	return nil
}
