package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"nexmap.net/internal/model"
	"nexmap.net/internal/store"
)

// deleteOwned deletes one entity that can carry comments, attachments and
// defect links, in one transaction with everything pointing at it and with
// its audit entry: the delete, the purge (see DiscussionService.PurgeOwner)
// and the audit entry happen together or not at all. The attachment bytes are
// unlinked after the commit.
//
// ownerType names the entity both as comment owner and in the audit trail.
func deleteOwned(ctx context.Context, t store.Transactor, purger ownerPurger, audit store.AuditStore,
	user *model.User, productID, ownerType, id string, del func(ctx context.Context, id string) error) error {
	var afterCommit func()
	err := inTx(ctx, t, func(ctx context.Context) error {
		var err error
		afterCommit, err = deleteOwnedInTx(ctx, purger, audit, user, productID, ownerType, id, del)
		return err
	})
	if err != nil {
		return err
	}
	if afterCommit != nil {
		afterCommit()
	}
	return nil
}

// deleteOwnedInTx is deleteOwned's body for a caller that already holds the
// transaction and deletes more than one entity in it. The returned function
// unlinks the attachment bytes and must run only after the commit.
func deleteOwnedInTx(ctx context.Context, purger ownerPurger, audit store.AuditStore,
	user *model.User, productID, ownerType, id string, del func(ctx context.Context, id string) error) (func(), error) {
	if err := del(ctx, id); err != nil {
		return nil, fmt.Errorf("delete %s: %w", strings.ReplaceAll(ownerType, "_", " "), err)
	}
	afterCommit, err := purger.PurgeOwner(ctx, productID, ownerType, id)
	if err != nil {
		return nil, err
	}
	return afterCommit, audit.Create(ctx, &model.AuditEntry{
		ID: uuid.New().String(), ProductID: productID, EntityType: ownerType, EntityID: id,
		UserID: user.ID, Action: "delete", CreatedAt: time.Now().UTC().Format(time.RFC3339),
	})
}
