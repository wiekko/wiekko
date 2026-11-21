// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package service

import (
	"context"
	"errors"
	"time"

	"wiekko.net/internal/store"
)

// inTx runs fn as one transaction through t (see store.Transactor). A nil t —
// a service built in a unit test on mock stores — runs fn directly: the mocks
// have nothing to roll back.
func inTx(ctx context.Context, t store.Transactor, fn func(ctx context.Context) error) error {
	if t == nil {
		return fn(ctx)
	}
	return t.InTx(ctx, fn)
}

// lock takes t's lock on key for the rest of the current transaction; with a
// nil t it does nothing.
func lock(ctx context.Context, t store.Transactor, key string) error {
	if t == nil {
		return nil
	}
	return t.Lock(ctx, key)
}

// Lock keys. Every transaction that reads a value and then writes based on it
// takes the key that guards that value, so two such transactions cannot both
// act on the same stale read.
func lockSequence(kind, productID string) string { return "seq:" + kind + ":" + productID }
func lockMembers(productID string) string        { return "members:" + productID }
func lockSprints(productID string) string        { return "sprints:" + productID }
func lockCategories(productID string) string     { return "categories:" + productID }
func lockCampaign(campaignID string) string      { return "campaign:" + campaignID }
func lockOwner(ownerID string) string            { return "owner:" + ownerID }
func lockEntity(id string) string                { return "entity:" + id }

const lockAdmins = "admins"

// ErrConflict is returned when an update was based on a version of the item
// that someone else has changed since.
var ErrConflict = errors.New("this item was changed by someone else since you opened it; reload to see the changes, then make yours again")

// checkVersion compares the updated_at a client edited (expected; "" when the
// client sent none) with the stored one.
func checkVersion(expected, current string) error {
	if expected == "" || sameInstant(expected, current) {
		return nil
	}
	return ErrConflict
}

// sameInstant compares two timestamps as instants, so that a client echoing a
// value in another (still RFC 3339) spelling is not taken for a conflict.
func sameInstant(a, b string) bool {
	if a == b {
		return true
	}
	ta, errA := time.Parse(time.RFC3339Nano, a)
	tb, errB := time.Parse(time.RFC3339Nano, b)
	return errA == nil && errB == nil && ta.Equal(tb)
}
