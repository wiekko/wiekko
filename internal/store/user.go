package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"

	"nexmap.net/internal/crypto"
	"nexmap.net/internal/model"
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
	Count(ctx context.Context) (int64, error)
}

type userStore struct {
	db   *sql.DB
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
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("user create: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	_, err = tx.ExecContext(ctx,
		`INSERT INTO users (id, username, password_hash, is_admin, status, totp_secret, totp_enabled, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		user.ID, user.Username, user.PasswordHash,
		boolToInt(user.IsAdmin), user.Status, user.TOTPSecret, boolToInt(user.TOTPEnabled),
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
	return s.getOne(ctx, `SELECT `+userSelectColumns+` `+userJoins+` WHERE u.username = ?`, username)
}

func (s *userStore) GetByEmail(ctx context.Context, email string) (*model.User, error) {
	blind := s.keys.BlindIndex(email)
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
		user.Username, boolToInt(user.IsAdmin), user.Status, user.UpdatedAt, user.ID,
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
	_, err := s.db.ExecContext(ctx, `UPDATE users SET status = ?, updated_at = datetime('now') WHERE id = ?`, status, id)
	return err
}

func (s *userStore) UpdatePassword(ctx context.Context, id, hash string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE users SET password_hash = ?, updated_at = datetime('now') WHERE id = ?`, hash, id)
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
	defer func(rows *sql.Rows) {
		err := rows.Close()
		if err != nil {
			slog.Error("failed to close rows", "err", err)
		}
	}(rows)

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

// scanner is satisfied by both *sql.Row and *sql.Rows.
type scanner interface {
	Scan(dest ...any) error
}

func (s *userStore) scanUser(sc scanner) (*model.User, error) {
	var u model.User
	var isAdmin, totpEnabled int
	var emailCT, emailKeyID, dnCT, dnKeyID sql.NullString

	err := sc.Scan(&u.ID, &u.Username, &u.PasswordHash,
		&isAdmin, &u.Status, &u.TOTPSecret, &totpEnabled, &u.CreatedAt, &u.UpdatedAt,
		&emailCT, &emailKeyID, &dnCT, &dnKeyID)
	if err != nil {
		return nil, err
	}

	u.IsAdmin = isAdmin != 0
	u.TOTPEnabled = totpEnabled != 0

	if emailCT.Valid && emailKeyID.Valid {
		u.Email, err = s.keys.Decrypt(emailCT.String, emailKeyID.String)
		if err != nil {
			return nil, fmt.Errorf("decrypt email: %w", err)
		}
	}

	if dnCT.Valid && dnKeyID.Valid {
		u.DisplayName, err = s.keys.Decrypt(dnCT.String, dnKeyID.String)
		if err != nil {
			return nil, fmt.Errorf("decrypt display_name: %w", err)
		}
	}

	return &u, nil
}

func (s *userStore) insertPersonalData(ctx context.Context, tx *sql.Tx, user *model.User) error {
	emailCT, emailKeyID, err := s.keys.Encrypt(user.Email)
	if err != nil {
		return fmt.Errorf("encrypt email: %w", err)
	}
	emailBlind := s.keys.BlindIndex(user.Email)

	_, err = tx.ExecContext(ctx,
		`INSERT INTO personal_data (id, owner_table, owner_id, field_name, ciphertext, key_id, blind_index)
		 VALUES (?, 'users', ?, 'email', ?, ?, ?)`,
		uuid.New().String(), user.ID, emailCT, emailKeyID, emailBlind,
	)
	if err != nil {
		return fmt.Errorf("insert email pd: %w", err)
	}

	dnCT, dnKeyID, err := s.keys.Encrypt(user.DisplayName)
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

func (s *userStore) updatePersonalData(ctx context.Context, tx *sql.Tx, user *model.User) error {
	emailCT, emailKeyID, err := s.keys.Encrypt(user.Email)
	if err != nil {
		return fmt.Errorf("encrypt email: %w", err)
	}
	emailBlind := s.keys.BlindIndex(user.Email)

	_, err = tx.ExecContext(ctx,
		`UPDATE personal_data SET ciphertext = ?, key_id = ?, blind_index = ?
		 WHERE owner_table = 'users' AND owner_id = ? AND field_name = 'email'`,
		emailCT, emailKeyID, emailBlind, user.ID,
	)
	if err != nil {
		return fmt.Errorf("update email pd: %w", err)
	}

	dnCT, dnKeyID, err := s.keys.Encrypt(user.DisplayName)
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

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
