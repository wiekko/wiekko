package client

import (
	"context"
	"net/http"
)

// Writes to the product structure, constraints and traceability links
// (FR-134, FR-135). Payloads carry IDs; resolving names and identifiers is
// the caller's job. Patch fields left nil are not changed; a pointer to ""
// clears an optional link.

// ComponentWrite is the payload of CreateComponent and UpdateComponent.
type ComponentWrite struct {
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
}

// CreateComponent creates a component (owner role, FR-79).
func (c *Client) CreateComponent(ctx context.Context, productID string, in ComponentWrite) (*Component, error) {
	return send[Component](ctx, c, http.MethodPost, productPath(productID, "components"), in)
}

// UpdateComponent patches a component.
func (c *Client) UpdateComponent(ctx context.Context, productID, id string, in ComponentWrite) (*Component, error) {
	return send[Component](ctx, c, http.MethodPatch, productPath(productID, "components", id), in)
}

// DeleteComponent deletes a component; its capabilities stay, without a
// component.
func (c *Client) DeleteComponent(ctx context.Context, productID, id string) error {
	return c.Do(ctx, http.MethodDelete, productPath(productID, "components", id), nil, nil)
}

// CapabilityWrite is the payload of CreateCapability and UpdateCapability.
type CapabilityWrite struct {
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
	ComponentID *string `json:"component_id,omitempty"`
}

// CreateCapability creates a capability (owner role, FR-79).
func (c *Client) CreateCapability(ctx context.Context, productID string, in CapabilityWrite) (*Capability, error) {
	return send[Capability](ctx, c, http.MethodPost, productPath(productID, "capabilities"), in)
}

// UpdateCapability patches a capability.
func (c *Client) UpdateCapability(ctx context.Context, productID, id string, in CapabilityWrite) (*Capability, error) {
	return send[Capability](ctx, c, http.MethodPatch, productPath(productID, "capabilities", id), in)
}

// DeleteCapability deletes a capability.
func (c *Client) DeleteCapability(ctx context.Context, productID, id string) error {
	return c.Do(ctx, http.MethodDelete, productPath(productID, "capabilities", id), nil, nil)
}

// LinkCapabilityRequirement links a requirement to a capability (FR-6).
func (c *Client) LinkCapabilityRequirement(ctx context.Context, productID, capabilityID, requirementID string) error {
	return c.Do(ctx, http.MethodPost, productPath(productID, "capabilities", capabilityID, "requirements"),
		map[string]string{"requirement_id": requirementID}, nil)
}

// UnlinkCapabilityRequirement removes that link.
func (c *Client) UnlinkCapabilityRequirement(ctx context.Context, productID, capabilityID, requirementID string) error {
	return c.Do(ctx, http.MethodDelete, productPath(productID, "capabilities", capabilityID, "requirements", requirementID), nil, nil)
}

// LinkConstraintCapability links a constraint to a capability (FR-44).
func (c *Client) LinkConstraintCapability(ctx context.Context, productID, constraintID, capabilityID string) error {
	return c.Do(ctx, http.MethodPost, productPath(productID, "constraints", constraintID, "capabilities"),
		map[string]string{"capability_id": capabilityID}, nil)
}

// UnlinkConstraintCapability removes that link.
func (c *Client) UnlinkConstraintCapability(ctx context.Context, productID, constraintID, capabilityID string) error {
	return c.Do(ctx, http.MethodDelete, productPath(productID, "constraints", constraintID, "capabilities", capabilityID), nil, nil)
}

// LinkTestCaseRequirement links a test case to a requirement (FR-22).
func (c *Client) LinkTestCaseRequirement(ctx context.Context, productID, testCaseID, requirementID string) error {
	return c.Do(ctx, http.MethodPost, productPath(productID, "test-cases", testCaseID, "requirements"),
		map[string]string{"requirement_id": requirementID}, nil)
}

// UnlinkTestCaseRequirement removes that link.
func (c *Client) UnlinkTestCaseRequirement(ctx context.Context, productID, testCaseID, requirementID string) error {
	return c.Do(ctx, http.MethodDelete, productPath(productID, "test-cases", testCaseID, "requirements", requirementID), nil, nil)
}

// TestCasePatch is a partial update of a test case.
type TestCasePatch struct {
	ComponentID  *string `json:"component_id,omitempty"`
	CapabilityID *string `json:"capability_id,omitempty"`
}

// UpdateTestCase patches a test case (developer role).
func (c *Client) UpdateTestCase(ctx context.Context, productID, id string, p TestCasePatch) (*TestCase, error) {
	return send[TestCase](ctx, c, http.MethodPatch, productPath(productID, "test-cases", id), p)
}

// ConstraintCreate is the payload of CreateConstraint.
type ConstraintCreate struct {
	CategoryID      string `json:"category_id,omitempty"`
	Title           string `json:"title"`
	Description     string `json:"description,omitempty"`
	ComplianceState string `json:"compliance_state,omitempty"`
}

// CreateConstraint creates a constraint (owner role, FR-104).
func (c *Client) CreateConstraint(ctx context.Context, productID string, in ConstraintCreate) (*Constraint, error) {
	return send[Constraint](ctx, c, http.MethodPost, productPath(productID, "constraints"), in)
}

// ConstraintPatch is a partial update of a constraint.
type ConstraintPatch struct {
	CategoryID      *string `json:"category_id,omitempty"`
	Title           *string `json:"title,omitempty"`
	Description     *string `json:"description,omitempty"`
	ComplianceState *string `json:"compliance_state,omitempty"`
}

// UpdateConstraint patches a constraint by identifier or ID.
func (c *Client) UpdateConstraint(ctx context.Context, productID, ref string, p ConstraintPatch) (*Constraint, error) {
	return send[Constraint](ctx, c, http.MethodPatch, productPath(productID, "constraints", ref), p)
}
