package client

import (
	"context"
	"net/http"
)

// Write calls. POST requests are never retried (see isIdempotent), so a
// create is sent at most once even when the connection drops.

func send[T any](ctx context.Context, c *Client, method, path string, body any) (*T, error) {
	var out T
	if err := c.Do(ctx, method, path, body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// RequirementCreate is the payload of CreateRequirement. IDs, not names:
// resolving names is the caller's job.
type RequirementCreate struct {
	Kind             string `json:"kind,omitempty"`
	ParentID         string `json:"parent_id,omitempty"`
	CategoryID       string `json:"category_id,omitempty"`
	Title            string `json:"title"`
	Description      string `json:"description,omitempty"`
	DefinitionOfDone string `json:"definition_of_done,omitempty"`
	StatusID         string `json:"status_id,omitempty"`
}

// CreateRequirement creates a requirement or task (owner role).
func (c *Client) CreateRequirement(ctx context.Context, productID string, in RequirementCreate) (*Requirement, error) {
	return send[Requirement](ctx, c, http.MethodPost, productPath(productID, "requirements"), in)
}

// RequirementPatch is a partial update; nil fields are left unchanged.
type RequirementPatch struct {
	StatusID *string `json:"status_id,omitempty"`
	IsActive *bool   `json:"is_active,omitempty"`
}

// UpdateRequirement patches a requirement or task by identifier or ID.
// Developers may change status and activation only (FR-407).
func (c *Client) UpdateRequirement(ctx context.Context, productID, ref string, p RequirementPatch) (*Requirement, error) {
	return send[Requirement](ctx, c, http.MethodPatch, productPath(productID, "requirements", ref), p)
}

// DefectCreate is the payload of CreateDefect.
type DefectCreate struct {
	Title         string `json:"title"`
	Description   string `json:"description,omitempty"`
	Criticality   string `json:"criticality,omitempty"`
	StatusID      string `json:"status_id,omitempty"`
	EnvironmentID string `json:"environment_id,omitempty"`
}

// CreateDefect creates a defect (tester role).
func (c *Client) CreateDefect(ctx context.Context, productID string, in DefectCreate) (*Defect, error) {
	return send[Defect](ctx, c, http.MethodPost, productPath(productID, "defects"), in)
}

// DefectPatch is a partial update; nil fields are left unchanged.
type DefectPatch struct {
	StatusID *string `json:"status_id,omitempty"`
}

// UpdateDefect patches a defect. Developers may change its status.
func (c *Client) UpdateDefect(ctx context.Context, productID, id string, p DefectPatch) (*Defect, error) {
	return send[Defect](ctx, c, http.MethodPatch, productPath(productID, "defects", id), p)
}

// AddDefectLink links a defect to a requirement, task, test case, component
// or capability (targetType: requirement, test_case, component, capability).
func (c *Client) AddDefectLink(ctx context.Context, productID, defectID, targetType, targetID string) error {
	body := map[string]string{"target_type": targetType, "target_id": targetID}
	return c.Do(ctx, http.MethodPost, productPath(productID, "defects", defectID, "links"), body, nil)
}

// CreateComment adds a Markdown comment to an entity. ownerType is one of
// requirement, task, constraint, test_case, component, capability, milestone,
// defect.
func (c *Client) CreateComment(ctx context.Context, productID, ownerType, ownerID, body string) (*Comment, error) {
	in := map[string]string{"owner_type": ownerType, "owner_id": ownerID, "body": body}
	return send[Comment](ctx, c, http.MethodPost, productPath(productID, "comments"), in)
}
