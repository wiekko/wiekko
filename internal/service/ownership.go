// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"nexmap.net/internal/model"
	"nexmap.net/internal/store"
)

// Entity kinds the ownership checker knows. Where a kind doubles as a comment
// owner type or defect link target, it uses the same spelling.
const (
	kindRequirement  = model.OwnerTypeRequirement // "requirement"
	kindTask         = model.OwnerTypeTask        // "task"
	kindBacklogItem  = "backlog_item"             // requirement or task
	kindConstraint   = model.OwnerTypeConstraint
	kindTestCase     = model.OwnerTypeTestCase
	kindComponent    = model.OwnerTypeComponent
	kindCapability   = model.OwnerTypeCapability
	kindMilestone    = model.OwnerTypeMilestone
	kindDefect       = model.OwnerTypeDefect
	kindCampaign     = "campaign"
	kindSprint       = "sprint"
	kindStatus       = "status"
	kindPyramidLevel = "pyramid_level"
	kindEnvironment  = "runtime_environment"
	kindCategory     = "category"
	kindProduct      = "product"
	kindComment      = "comment"
	kindAttachment   = "attachment"
	kindDefectLink   = "defect_link"
	kindDocument     = model.OwnerTypeDocument
)

// ref names one entity for an ownership check. An empty ID is skipped, so
// optional references ("no milestone") can be passed unconditionally.
type ref struct {
	kind string
	id   string
}

// productOfFunc reports the product an entity of the given kind belongs to.
type productOfFunc func(ctx context.Context, kind, id string) (string, error)

// ownership enforces that every entity a request names belongs to the product
// in the URL. Authorization (Authorizer.RequireRole) answers "may this user
// act on product P?"; ownership answers "is entity X part of product P?".
// Without the second check, a member of P could reach any entity of any other
// product by putting its ID under P's URL.
//
// A mismatch is reported as ErrNotFound, never ErrForbidden, so a request
// cannot be used to probe which IDs exist elsewhere.
type ownership struct {
	productOf productOfFunc
}

// errOwnershipUnset fails closed when a service was built without a checker.
var errOwnershipUnset = errors.New("service: ownership checks are not configured")

// require checks that every non-empty ref belongs to productID.
func (o *ownership) require(ctx context.Context, productID string, refs ...ref) error {
	if o == nil || o.productOf == nil {
		return errOwnershipUnset
	}
	for _, r := range refs {
		if r.id == "" {
			continue
		}
		pid, err := o.productOf(ctx, r.kind, r.id)
		if err != nil || pid != productID {
			return fmt.Errorf("%w: %s %s", ErrNotFound, strings.ReplaceAll(r.kind, "_", " "), r.id)
		}
	}
	return nil
}

// newOwnership builds the production checker from the store.
func newOwnership(st *store.Store) *ownership {
	return &ownership{productOf: func(ctx context.Context, kind, id string) (string, error) {
		switch kind {
		case kindRequirement, kindTask, kindBacklogItem:
			r, err := st.Requirement.GetByID(ctx, id)
			if err != nil {
				return "", err
			}
			// A comment filed under "task" must name a task, and vice versa.
			if kind != kindBacklogItem && r.Kind != kind {
				return "", ErrNotFound
			}
			return r.ProductID, nil
		case kindConstraint:
			return productIDOf(st.Constraint.GetByID(ctx, id))
		case kindTestCase:
			return productIDOf(st.TestCase.GetByID(ctx, id))
		case kindComponent:
			return productIDOf(st.Component.GetByID(ctx, id))
		case kindCapability:
			return productIDOf(st.Capability.GetByID(ctx, id))
		case kindMilestone:
			return productIDOf(st.Milestone.GetByID(ctx, id))
		case kindDefect:
			return productIDOf(st.Defect.GetByID(ctx, id))
		case kindDocument:
			return productIDOf(st.Document.GetByID(ctx, id))
		case kindCampaign:
			return productIDOf(st.TestingCampaign.GetByID(ctx, id))
		case kindSprint:
			return productIDOf(st.Sprint.GetByID(ctx, id))
		case kindStatus:
			return productIDOf(st.Status.GetByID(ctx, id))
		case kindPyramidLevel:
			return productIDOf(st.PyramidLevel.GetByID(ctx, id))
		case kindEnvironment:
			return productIDOf(st.RuntimeEnvironment.GetByID(ctx, id))
		case kindCategory:
			return productIDOf(st.Category.GetByID(ctx, id))
		case kindDefectLink:
			l, err := st.Defect.GetLink(ctx, id)
			if err != nil {
				return "", err
			}
			return productIDOf(st.Defect.GetByID(ctx, l.DefectID))
		case kindComment:
			return productIDOf(st.Comment.GetByID(ctx, id))
		case kindAttachment:
			return productIDOf(st.Attachment.GetByID(ctx, id))
		case kindProduct:
			return id, nil
		}
		return "", fmt.Errorf("unknown entity kind %q", kind)
	}}
}

// productScoped is any model with a ProductID; productIDOf adapts a store
// lookup to productOfFunc.
type productScoped interface {
	*model.Requirement | *model.Constraint | *model.TestCase | *model.Component | *model.Capability |
		*model.Milestone | *model.Defect | *model.TestingCampaign | *model.Sprint | *model.Status |
		*model.PyramidLevel | *model.RuntimeEnvironment | *model.Category | *model.Comment | *model.Attachment |
		*model.Document
}

func productIDOf[T productScoped](entity T, err error) (string, error) {
	if err != nil {
		return "", err
	}
	switch e := any(entity).(type) {
	case *model.Requirement:
		return e.ProductID, nil
	case *model.Constraint:
		return e.ProductID, nil
	case *model.TestCase:
		return e.ProductID, nil
	case *model.Component:
		return e.ProductID, nil
	case *model.Capability:
		return e.ProductID, nil
	case *model.Milestone:
		return e.ProductID, nil
	case *model.Defect:
		return e.ProductID, nil
	case *model.TestingCampaign:
		return e.ProductID, nil
	case *model.Sprint:
		return e.ProductID, nil
	case *model.Status:
		return e.ProductID, nil
	case *model.PyramidLevel:
		return e.ProductID, nil
	case *model.RuntimeEnvironment:
		return e.ProductID, nil
	case *model.Category:
		return e.ProductID, nil
	case *model.Comment:
		return e.ProductID, nil
	case *model.Attachment:
		return e.ProductID, nil
	case *model.Document:
		return e.ProductID, nil
	}
	return "", fmt.Errorf("no product for %T", entity)
}

// auditEntityKinds maps the entity types recorded in the audit trail to
// ownership kinds. An audit query for a type missing here is refused.
var auditEntityKinds = map[string]string{
	"product":             kindProduct,
	"component":           kindComponent,
	"capability":          kindCapability,
	"requirement":         kindBacklogItem,
	"task":                kindBacklogItem,
	"constraint":          kindConstraint,
	"test_case":           kindTestCase,
	"defect":              kindDefect,
	"document":            kindDocument,
	"milestone":           kindMilestone,
	"sprint":              kindSprint,
	"testing_campaign":    kindCampaign,
	"category":            kindCategory,
	"status":              kindStatus,
	"pyramid_level":       kindPyramidLevel,
	"runtime_environment": kindEnvironment,
	"comment":             kindComment,
	"attachment":          kindAttachment,
}

// defectLinkKind maps a defect link target type to its ownership kind. Defects
// may be linked to requirements and tasks alike.
func defectLinkKind(targetType string) string {
	if targetType == model.DefectLinkTargetRequirement {
		return kindBacklogItem
	}
	return targetType
}

// opt turns an optional (pointer) reference from an update input into a ref
// ID; nil means "unchanged" and is skipped like an empty ID.
func opt(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
