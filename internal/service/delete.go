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
		if err := del(ctx, id); err != nil {
			return fmt.Errorf("delete %s: %w", strings.ReplaceAll(ownerType, "_", " "), err)
		}
		var err error
		if afterCommit, err = purger.PurgeOwner(ctx, productID, ownerType, id); err != nil {
			return err
		}
		return audit.Create(ctx, &model.AuditEntry{
			ID: uuid.New().String(), EntityType: ownerType, EntityID: id,
			UserID: user.ID, Action: "delete", CreatedAt: time.Now().UTC().Format(time.RFC3339),
		})
	})
	if err != nil {
		return err
	}
	if afterCommit != nil {
		afterCommit()
	}
	return nil
}
