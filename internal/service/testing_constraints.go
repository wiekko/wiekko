// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package service

import (
	"context"
	"sort"
	"time"

	"github.com/google/uuid"

	"nexmap.net/internal/model"
)

// Test case to constraint links (FR-150). A test that verifies a constraint,
// or part of it, is evidence for the constraint's compliance; compliance
// itself stays a decision of the product owner.

// TestCaseEvidence is a test case with its latest result.
type TestCaseEvidence struct {
	TestCase *model.TestCase
	// Latest is the most recently executed result of the test case in any
	// campaign and environment; nil if it has never been executed.
	Latest *model.CampaignResult
}

// DerivedTestCase is a test case reached through capabilities linked to a
// constraint (UC-87). It is not linked to the constraint itself.
type DerivedTestCase struct {
	TestCaseEvidence
	// Via are the constraint's capabilities the test case belongs to.
	Via []*model.Capability
}

// ConstraintTestCases are the test cases of a constraint: the ones linked to
// it, and, read-only, the ones of its capabilities.
type ConstraintTestCases struct {
	Linked  []TestCaseEvidence
	Derived []DerivedTestCase
}

// LinkConstraint links a test case to a constraint; linking a linked pair
// again changes nothing. Requires developer role, like requirement links.
func (s *TestingService) LinkConstraint(ctx context.Context, user *model.User, productID, tcID, constraintID string) error {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleDeveloper); err != nil {
		return err
	}
	if err := s.owns.require(ctx, productID, ref{kindTestCase, tcID}, ref{kindConstraint, constraintID}); err != nil {
		return err
	}
	return s.testCases.LinkConstraint(ctx, &model.TestCaseConstraint{
		ID:           uuid.New().String(),
		TestCaseID:   tcID,
		ConstraintID: constraintID,
		CreatedAt:    time.Now().UTC().Format(time.RFC3339),
	})
}

// UnlinkConstraint removes the link between a test case and a constraint.
// Requires developer role.
func (s *TestingService) UnlinkConstraint(ctx context.Context, user *model.User, productID, tcID, constraintID string) error {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleDeveloper); err != nil {
		return err
	}
	if err := s.owns.require(ctx, productID, ref{kindTestCase, tcID}, ref{kindConstraint, constraintID}); err != nil {
		return err
	}
	return s.testCases.UnlinkConstraint(ctx, tcID, constraintID)
}

// ListTestCaseConstraints returns the constraints a test case is linked to,
// by sequence number. Requires viewer role.
func (s *TestingService) ListTestCaseConstraints(ctx context.Context, user *model.User, productID, tcID string) ([]*model.Constraint, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleViewer); err != nil {
		return nil, err
	}
	if err := s.owns.require(ctx, productID, ref{kindTestCase, tcID}); err != nil {
		return nil, err
	}
	links, err := s.testCases.ListConstraintLinks(ctx, tcID)
	if err != nil {
		return nil, err
	}
	out := make([]*model.Constraint, 0, len(links))
	for _, link := range links {
		c, err := s.constraints.GetByID(ctx, link.ConstraintID)
		if err != nil {
			return nil, err
		}
		if c.ProductID == productID {
			out = append(out, c)
		}
	}
	sort.SliceStable(out, func(a, b int) bool { return out[a].SequenceNumber < out[b].SequenceNumber })
	return out, nil
}

// ListConstraintTestCases returns the test cases linked to a constraint, and
// those of the constraint's capabilities that are not linked to it, each by
// title and with its latest result. Requires viewer role.
func (s *TestingService) ListConstraintTestCases(ctx context.Context, user *model.User, productID, constraintID string) (*ConstraintTestCases, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleViewer); err != nil {
		return nil, err
	}
	if err := s.owns.require(ctx, productID, ref{kindConstraint, constraintID}); err != nil {
		return nil, err
	}

	out := &ConstraintTestCases{Linked: []TestCaseEvidence{}, Derived: []DerivedTestCase{}}
	links, err := s.testCases.ListLinksByConstraint(ctx, constraintID)
	if err != nil {
		return nil, err
	}
	linked := make(map[string]bool, len(links))
	for _, link := range links {
		tc, err := s.testCases.GetByID(ctx, link.TestCaseID)
		if err != nil {
			return nil, err
		}
		if tc.ProductID != productID {
			continue
		}
		ev, err := s.evidence(ctx, tc)
		if err != nil {
			return nil, err
		}
		linked[tc.ID] = true
		out.Linked = append(out.Linked, ev)
	}

	capLinks, err := s.constraints.ListCapabilitiesByConstraint(ctx, constraintID)
	if err != nil {
		return nil, err
	}
	index := map[string]int{}
	for _, capLink := range capLinks {
		capability, err := s.capabilities.GetByID(ctx, capLink.CapabilityID)
		if err != nil {
			return nil, err
		}
		cases, err := s.testCases.ListByCapabilityID(ctx, capability.ID)
		if err != nil {
			return nil, err
		}
		for _, tc := range cases {
			if linked[tc.ID] || tc.ProductID != productID {
				continue
			}
			i, seen := index[tc.ID]
			if !seen {
				ev, err := s.evidence(ctx, tc)
				if err != nil {
					return nil, err
				}
				i = len(out.Derived)
				index[tc.ID] = i
				out.Derived = append(out.Derived, DerivedTestCase{TestCaseEvidence: ev})
			}
			out.Derived[i].Via = append(out.Derived[i].Via, capability)
		}
	}
	sort.SliceStable(out.Linked, func(a, b int) bool {
		return out.Linked[a].TestCase.Title < out.Linked[b].TestCase.Title
	})
	sort.SliceStable(out.Derived, func(a, b int) bool {
		return out.Derived[a].TestCase.Title < out.Derived[b].TestCase.Title
	})
	return out, nil
}

// evidence pairs a test case with its latest result.
func (s *TestingService) evidence(ctx context.Context, tc *model.TestCase) (TestCaseEvidence, error) {
	results, err := s.campaignResults.ListByTestCaseID(ctx, tc.ID)
	if err != nil {
		return TestCaseEvidence{}, err
	}
	return TestCaseEvidence{TestCase: tc, Latest: LatestResult(results)}, nil
}

// LatestResult returns the most recently executed of results, nil if none
// was executed yet. Pending results have no execution time and never count.
func LatestResult(results []*model.CampaignResult) *model.CampaignResult {
	var latest *model.CampaignResult
	var latestAt time.Time
	for _, r := range results {
		if r.ExecutedAt == "" || r.Status == model.CampaignResultStatusPending {
			continue
		}
		at, err := time.Parse(time.RFC3339, r.ExecutedAt)
		if err != nil {
			continue
		}
		if latest == nil || at.After(latestAt) {
			latest, latestAt = r, at
		}
	}
	return latest
}
