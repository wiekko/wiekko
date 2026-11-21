// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// Transactor runs several store calls as one unit. Services receive it next to
// their stores and pass the context InTx hands them to every call that belongs
// to the unit.
type Transactor interface {
	// InTx runs fn in a transaction. Every store call made with the context fn
	// receives joins it; the transaction commits when fn returns nil and rolls
	// back otherwise. Calling InTx with a context that already carries a
	// transaction joins that transaction instead of starting a new one.
	//
	// Inside fn, never discard the error of a write (`_ = store.Create(...)`):
	// on Postgres a failed statement aborts the whole transaction, so the
	// swallowed error would only resurface as a failed commit, and on SQLite
	// the half-done unit would commit. A write that may fail without failing
	// the unit belongs after InTx returns.
	InTx(ctx context.Context, fn func(ctx context.Context) error) error

	// Lock serializes transactions that take the same key until the current
	// transaction ends. It must be called inside InTx. Use it where a decision
	// read from the database must still hold when the write lands: drawing
	// the next sequence number, keeping the last owner, one active sprint.
	Lock(ctx context.Context, key string) error
}

type txKey struct{}

// txFrom returns the transaction ctx carries, if any.
func txFrom(ctx context.Context) *Tx {
	if ctx == nil {
		return nil
	}
	tx, _ := ctx.Value(txKey{}).(*Tx)
	return tx
}

// InTx implements Transactor.
func (d *DB) InTx(ctx context.Context, fn func(ctx context.Context) error) (err error) {
	if txFrom(ctx) != nil {
		return fn(ctx)
	}
	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p)
		}
		if err != nil {
			_ = tx.Rollback()
			return
		}
		if cerr := tx.Commit(); cerr != nil {
			err = fmt.Errorf("commit transaction: %w", cerr)
		}
	}()
	return fn(context.WithValue(ctx, txKey{}, tx))
}

// Lock implements Transactor. On SQLite it does nothing: the pool holds a
// single connection (see database.openSQLite), so transactions already run one
// after another.
func (d *DB) Lock(ctx context.Context, key string) error {
	tx := txFrom(ctx)
	if tx == nil {
		return fmt.Errorf("lock %q: not inside a transaction", key)
	}
	if d.dialect != Postgres {
		return nil
	}
	_, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext(?))`, key)
	if err != nil {
		return fmt.Errorf("lock %q: %w", key, err)
	}
	return nil
}

// ErrNotChanged is returned by a conditional write whose condition no longer
// held: the row was changed, used up or removed since it was read.
var ErrNotChanged = errors.New("the row changed since it was read")

// requireChanged turns a conditional write that touched no row into
// ErrNotChanged.
func requireChanged(res sql.Result, err error) error {
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotChanged
	}
	return nil
}
