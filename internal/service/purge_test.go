package service

import (
	"context"
	"errors"
	"testing"

	"nexmap.net/internal/model"
	"nexmap.net/internal/testutil"
)

// recordingPurger stands in for DiscussionService in entity-service tests and
// records the cascade calls made against it.
type recordingPurger struct {
	owners   []purgedOwner
	products []string
	err      error
}

type purgedOwner struct {
	productID string
	ownerType string
	ownerID   string
}

func (p *recordingPurger) PurgeOwner(_ context.Context, productID, ownerType, ownerID string) error {
	p.owners = append(p.owners, purgedOwner{productID, ownerType, ownerID})
	return p.err
}

func (p *recordingPurger) PurgeProduct(_ context.Context, productID string) error {
	p.products = append(p.products, productID)
	return p.err
}

func deletesOK() func(_ context.Context, _ string) error {
	return func(_ context.Context, _ string) error { return nil }
}

// TestDeleteCascadesDiscussion covers every entity that can carry comments and
// attachments. Their owner columns are polymorphic, so nothing in SQL cascades
// the thread away — each delete has to ask for it, and forgetting to left the
// rows (and the attachment blobs) orphaned.
func TestDeleteCascadesDiscussion(t *testing.T) {
	cases := []struct {
		name      string
		ownerType string
		del       func(purger *recordingPurger) error
	}{
		{
			name:      "component",
			ownerType: model.OwnerTypeComponent,
			del: func(p *recordingPurger) error {
				svc := &ComponentService{
					components: &testutil.MockComponentStore{DeleteFunc: deletesOK()},
					comments:   p,
					audit:      noopAudit(),
					authz:      &Authorizer{productRoles: ownerRoles()},
				}
				return svc.DeleteComponent(context.Background(), testutil.MakeUser(), "product-001", "entity-001")
			},
		},
		{
			name:      "capability",
			ownerType: model.OwnerTypeCapability,
			del: func(p *recordingPurger) error {
				svc := &ComponentService{
					capabilities: &testutil.MockCapabilityStore{DeleteFunc: deletesOK()},
					comments:     p,
					audit:        noopAudit(),
					authz:        &Authorizer{productRoles: ownerRoles()},
				}
				return svc.DeleteCapability(context.Background(), testutil.MakeUser(), "product-001", "entity-001")
			},
		},
		{
			name:      "milestone",
			ownerType: model.OwnerTypeMilestone,
			del: func(p *recordingPurger) error {
				svc := &MilestoneService{
					milestones: &testutil.MockMilestoneStore{DeleteFunc: deletesOK()},
					comments:   p,
					audit:      noopAudit(),
					authz:      &Authorizer{productRoles: ownerRoles()},
				}
				return svc.DeleteMilestone(context.Background(), testutil.MakeUser(), "product-001", "entity-001")
			},
		},
		{
			name:      "requirement",
			ownerType: model.OwnerTypeRequirement,
			del: func(p *recordingPurger) error {
				svc := &BacklogService{
					requirements: &testutil.MockRequirementStore{DeleteFunc: deletesOK()},
					comments:     p,
					audit:        noopAudit(),
					authz:        &Authorizer{productRoles: ownerRoles()},
				}
				return svc.DeleteRequirement(context.Background(), testutil.MakeUser(), "product-001", "entity-001")
			},
		},
		{
			name:      "defect",
			ownerType: model.OwnerTypeDefect,
			del: func(p *recordingPurger) error {
				svc := &BacklogService{
					defects:  &testutil.MockDefectStore{DeleteFunc: deletesOK()},
					comments: p,
					audit:    noopAudit(),
					authz:    &Authorizer{productRoles: ownerRoles()},
				}
				return svc.DeleteDefect(context.Background(), testutil.MakeUser(), "product-001", "entity-001")
			},
		},
		{
			name:      "constraint",
			ownerType: model.OwnerTypeConstraint,
			del: func(p *recordingPurger) error {
				svc := &ConstraintService{
					constraints: &testutil.MockConstraintStore{DeleteFunc: deletesOK()},
					comments:    p,
					audit:       noopAudit(),
					authz:       &Authorizer{productRoles: ownerRoles()},
				}
				return svc.DeleteConstraint(context.Background(), testutil.MakeUser(), "product-001", "entity-001")
			},
		},
		{
			name:      "test case",
			ownerType: model.OwnerTypeTestCase,
			del: func(p *recordingPurger) error {
				svc := &TestingService{
					testCases: &testutil.MockTestCaseStore{DeleteFunc: deletesOK()},
					comments:  p,
					audit:     noopAudit(),
					authz:     &Authorizer{productRoles: ownerRoles()},
				}
				return svc.DeleteTestCase(context.Background(), testutil.MakeUser(), "product-001", "entity-001")
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			purger := &recordingPurger{}
			if err := tc.del(purger); err != nil {
				t.Fatalf("delete %s: %v", tc.name, err)
			}

			want := purgedOwner{"product-001", tc.ownerType, "entity-001"}
			if len(purger.owners) != 1 || purger.owners[0] != want {
				t.Fatalf("purged %v, want exactly [%v]", purger.owners, want)
			}
		})
	}

	t.Run("a failed purge is reported, not swallowed", func(t *testing.T) {
		sentinel := errors.New("purge exploded")
		purger := &recordingPurger{err: sentinel}
		svc := &ComponentService{
			components: &testutil.MockComponentStore{DeleteFunc: deletesOK()},
			comments:   purger,
			audit:      noopAudit(),
			authz:      &Authorizer{productRoles: ownerRoles()},
		}

		err := svc.DeleteComponent(context.Background(), testutil.MakeUser(), "product-001", "entity-001")
		if !errors.Is(err, sentinel) {
			t.Fatalf("err = %v, want %v", err, sentinel)
		}
	})
}

// TestDeleteProductPurgesBlobs covers the one case SQL half-handles: the rows
// cascade with the product, but the stored bytes need unlinking by hand.
func TestDeleteProductPurgesBlobs(t *testing.T) {
	purger := &recordingPurger{}
	svc := &ProductService{
		products:    &testutil.MockProductStore{DeleteFunc: deletesOK()},
		attachments: purger,
		audit:       noopAudit(),
		authz:       &Authorizer{productRoles: ownerRoles()},
	}

	if err := svc.Delete(context.Background(), testutil.MakeUser(), "product-001"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if len(purger.products) != 1 || purger.products[0] != "product-001" {
		t.Fatalf("purged %v, want exactly [product-001]", purger.products)
	}
}
