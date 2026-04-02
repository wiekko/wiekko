package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"

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
	db *sql.DB
}

func (s *userStore) Create(ctx context.Context, user *model.User) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO users (id, username, email, display_name, password_hash, is_admin, status, totp_secret, totp_enabled, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		user.ID, user.Username, user.Email, user.DisplayName, user.PasswordHash,
		boolToInt(user.IsAdmin), user.Status, user.TOTPSecret, boolToInt(user.TOTPEnabled),
		user.CreatedAt, user.UpdatedAt,
	)
	return err
}

func (s *userStore) GetByID(ctx context.Context, id string) (*model.User, error) {
	return s.getOne(ctx, `SELECT id, username, email, display_name, password_hash, is_admin, status, totp_secret, totp_enabled, created_at, updated_at FROM users WHERE id = ?`, id)
}

func (s *userStore) GetByUsername(ctx context.Context, username string) (*model.User, error) {
	return s.getOne(ctx, `SELECT id, username, email, display_name, password_hash, is_admin, status, totp_secret, totp_enabled, created_at, updated_at FROM users WHERE username = ?`, username)
}

func (s *userStore) GetByEmail(ctx context.Context, email string) (*model.User, error) {
	return s.getOne(ctx, `SELECT id, username, email, display_name, password_hash, is_admin, status, totp_secret, totp_enabled, created_at, updated_at FROM users WHERE email = ?`, email)
}

func (s *userStore) Update(ctx context.Context, user *model.User) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE users SET username = ?, email = ?, display_name = ?, is_admin = ?, status = ?, updated_at = ? WHERE id = ?`,
		user.Username, user.Email, user.DisplayName, boolToInt(user.IsAdmin), user.Status, user.UpdatedAt, user.ID,
	)
	return err
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
	_, err := s.db.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, id)
	return err
}

func (s *userStore) List(ctx context.Context, filter UserFilter) ([]*model.User, error) {
	query := `SELECT id, username, email, display_name, password_hash, is_admin, status, totp_secret, totp_enabled, created_at, updated_at FROM users`
	var args []any
	if filter.Status != "" {
		query += ` WHERE status = ?`
		args = append(args, filter.Status)
	}
	query += ` ORDER BY username`

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
		u, err := scanUser(rows)
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
	u, err := scanUser(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("user not found: %w", err)
	}
	return u, err
}

// scanner is satisfied by both *sql.Row and *sql.Rows.
type scanner interface {
	Scan(dest ...any) error
}

func scanUser(s scanner) (*model.User, error) {
	var u model.User
	var isAdmin, totpEnabled int
	err := s.Scan(&u.ID, &u.Username, &u.Email, &u.DisplayName, &u.PasswordHash,
		&isAdmin, &u.Status, &u.TOTPSecret, &totpEnabled, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, err
	}
	u.IsAdmin = isAdmin != 0
	u.TOTPEnabled = totpEnabled != 0
	return &u, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
