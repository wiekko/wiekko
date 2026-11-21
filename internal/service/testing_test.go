// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package service

import (
	"context"
	"errors"
	"testing"

	"wiekko.net/internal/model"
	"wiekko.net/internal/testutil"
)

func newTestingService(
	testCases *testutil.MockTestCaseStore,
	campaigns *testutil.MockTestingCampaignStore,
	campaignResults *testutil.MockCampaignResultStore,
	audit *testutil.MockAuditStore,
	roles *testutil.MockProductRoleStore,
) *TestingService {
	return &TestingService{
		owns:            testOwnership(),
		testCases:       testCases,
		campaigns:       campaigns,
		campaignResults: campaignResults,
		// Events are fire-and-forget; tests that assert on them replace this.
		campaignEvents: &testutil.MockCampaignResultEventStore{},
		comments:       &recordingPurger{},
		audit:          audit,
		authz:          &Authorizer{productRoles: roles},
	}
}

// activeCampaign returns a campaign store whose GetByID yields an active
// campaign, the state in which results are writable (FR-124).
func activeCampaign() *testutil.MockTestingCampaignStore {
	return &testutil.MockTestingCampaignStore{
		GetByIDFunc: func(_ context.Context, id string) (*model.TestingCampaign, error) {
			return testutil.MakeTestingCampaign(func(c *model.TestingCampaign) {
				c.ID = id
				c.Status = model.CampaignStatusActive
			}), nil
		},
	}
}

// --- Test Cases ---

func TestCreateTestCase(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		var created *model.TestCase
		svc := newTestingService(
			&testutil.MockTestCaseStore{
				CreateFunc: func(_ context.Context, tc *model.TestCase) error { created = tc; return nil },
			},
			nil, nil, noopAudit(), developerRoles(),
		)

		result, err := svc.CreateTestCase(context.Background(), testutil.MakeUser(), CreateTestCaseInput{
			ProductID: "product-001", Title: "Login test",
		})
		if err != nil {
			t.Fatalf("CreateTestCase: %v", err)
		}
		if result.Title != "Login test" {
			t.Fatalf("Title = %q, want %q", result.Title, "Login test")
		}
		if created == nil {
			t.Fatal("test case was not created")
		}
		if created.ExecutionMode != model.ExecutionModeManual {
			t.Fatalf("ExecutionMode = %q, want %q", created.ExecutionMode, model.ExecutionModeManual)
		}
	})

	t.Run("missing title", func(t *testing.T) {
		svc := newTestingService(nil, nil, nil, nil, developerRoles())
		_, err := svc.CreateTestCase(context.Background(), testutil.MakeUser(), CreateTestCaseInput{ProductID: "product-001"})
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newTestingService(nil, nil, nil, nil, forbiddenRoles())
		_, err := svc.CreateTestCase(context.Background(), testutil.MakeUser(), CreateTestCaseInput{ProductID: "product-001", Title: "X"})
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

func TestGetTestCase(t *testing.T) {
	tc := testutil.MakeTestCase()

	t.Run("success", func(t *testing.T) {
		svc := newTestingService(
			&testutil.MockTestCaseStore{
				GetByIDFunc: func(_ context.Context, _ string) (*model.TestCase, error) { return tc, nil },
			},
			nil, nil, nil, viewerRoles(),
		)

		result, err := svc.GetTestCase(context.Background(), testutil.MakeUser(), "product-001", "tc-001")
		if err != nil {
			t.Fatalf("GetTestCase: %v", err)
		}
		if result.ID != tc.ID {
			t.Fatalf("ID = %q, want %q", result.ID, tc.ID)
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newTestingService(nil, nil, nil, nil, forbiddenRoles())
		_, err := svc.GetTestCase(context.Background(), testutil.MakeUser(), "product-001", "tc-001")
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

func TestUpdateTestCase(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		tc := testutil.MakeTestCase()
		var updated *model.TestCase
		svc := newTestingService(
			&testutil.MockTestCaseStore{
				GetByIDFunc: func(_ context.Context, _ string) (*model.TestCase, error) { return tc, nil },
				UpdateFunc:  func(_ context.Context, tc *model.TestCase) error { updated = tc; return nil },
			},
			nil, nil, noopAudit(), developerRoles(),
		)

		newTitle := "Updated"
		result, err := svc.UpdateTestCase(context.Background(), testutil.MakeUser(), "product-001", "tc-001", UpdateTestCaseInput{Title: &newTitle})
		if err != nil {
			t.Fatalf("UpdateTestCase: %v", err)
		}
		if result.Title != "Updated" {
			t.Fatalf("Title = %q, want %q", result.Title, "Updated")
		}
		if updated == nil {
			t.Fatal("test case was not updated")
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newTestingService(nil, nil, nil, nil, forbiddenRoles())
		_, err := svc.UpdateTestCase(context.Background(), testutil.MakeUser(), "product-001", "tc-001", UpdateTestCaseInput{})
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

func TestDeleteTestCase(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		deleted := false
		svc := newTestingService(
			&testutil.MockTestCaseStore{
				DeleteFunc: func(_ context.Context, _ string) error { deleted = true; return nil },
			},
			nil, nil, noopAudit(), developerRoles(),
		)

		err := svc.DeleteTestCase(context.Background(), testutil.MakeUser(), "product-001", "tc-001")
		if err != nil {
			t.Fatalf("DeleteTestCase: %v", err)
		}
		if !deleted {
			t.Fatal("test case was not deleted")
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newTestingService(nil, nil, nil, nil, forbiddenRoles())
		err := svc.DeleteTestCase(context.Background(), testutil.MakeUser(), "product-001", "tc-001")
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

func TestListTestCases(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		svc := newTestingService(
			&testutil.MockTestCaseStore{
				ListByProductIDFunc: func(_ context.Context, _ string) ([]*model.TestCase, error) {
					return []*model.TestCase{testutil.MakeTestCase()}, nil
				},
			},
			nil, nil, nil, viewerRoles(),
		)

		result, err := svc.ListTestCases(context.Background(), testutil.MakeUser(), "product-001")
		if err != nil {
			t.Fatalf("ListTestCases: %v", err)
		}
		if len(result) != 1 {
			t.Fatalf("len(result) = %d, want 1", len(result))
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newTestingService(nil, nil, nil, nil, forbiddenRoles())
		_, err := svc.ListTestCases(context.Background(), testutil.MakeUser(), "product-001")
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

func TestLinkRequirement(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		linked := false
		svc := newTestingService(
			&testutil.MockTestCaseStore{
				LinkRequirementFunc: func(_ context.Context, _ *model.TestCaseRequirement) error { linked = true; return nil },
			},
			nil, nil, nil, developerRoles(),
		)

		err := svc.LinkRequirement(context.Background(), testutil.MakeUser(), "product-001", "tc-001", "req-001", "requirement")
		if err != nil {
			t.Fatalf("LinkRequirement: %v", err)
		}
		if !linked {
			t.Fatal("link was not created")
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newTestingService(nil, nil, nil, nil, forbiddenRoles())
		err := svc.LinkRequirement(context.Background(), testutil.MakeUser(), "product-001", "tc-001", "req-001", "requirement")
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

func TestUnlinkRequirement(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		unlinked := false
		svc := newTestingService(
			&testutil.MockTestCaseStore{
				UnlinkRequirementFunc: func(_ context.Context, _, _ string) error { unlinked = true; return nil },
			},
			nil, nil, nil, developerRoles(),
		)

		err := svc.UnlinkRequirement(context.Background(), testutil.MakeUser(), "product-001", "tc-001", "req-001")
		if err != nil {
			t.Fatalf("UnlinkRequirement: %v", err)
		}
		if !unlinked {
			t.Fatal("link was not removed")
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newTestingService(nil, nil, nil, nil, forbiddenRoles())
		err := svc.UnlinkRequirement(context.Background(), testutil.MakeUser(), "product-001", "tc-001", "req-001")
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

func TestListTestCaseRequirements(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		svc := newTestingService(
			&testutil.MockTestCaseStore{
				ListRequirementLinksFunc: func(_ context.Context, _ string) ([]*model.TestCaseRequirement, error) {
					return []*model.TestCaseRequirement{{ID: "tcr-001"}}, nil
				},
			},
			nil, nil, nil, viewerRoles(),
		)

		result, err := svc.ListTestCaseRequirements(context.Background(), testutil.MakeUser(), "product-001", "tc-001")
		if err != nil {
			t.Fatalf("ListTestCaseRequirements: %v", err)
		}
		if len(result) != 1 {
			t.Fatalf("len(result) = %d, want 1", len(result))
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newTestingService(nil, nil, nil, nil, forbiddenRoles())
		_, err := svc.ListTestCaseRequirements(context.Background(), testutil.MakeUser(), "product-001", "tc-001")
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

func TestAddTestCaseEnvironment(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		added := false
		svc := newTestingService(
			&testutil.MockTestCaseStore{
				AddEnvironmentFunc: func(_ context.Context, _, _, _ string) error { added = true; return nil },
			},
			nil, nil, nil, developerRoles(),
		)

		err := svc.AddTestCaseEnvironment(context.Background(), testutil.MakeUser(), "product-001", "tc-001", "env-001")
		if err != nil {
			t.Fatalf("AddTestCaseEnvironment: %v", err)
		}
		if !added {
			t.Fatal("environment was not added")
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newTestingService(nil, nil, nil, nil, forbiddenRoles())
		err := svc.AddTestCaseEnvironment(context.Background(), testutil.MakeUser(), "product-001", "tc-001", "env-001")
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

func TestRemoveTestCaseEnvironment(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		removed := false
		svc := newTestingService(
			&testutil.MockTestCaseStore{
				RemoveEnvironmentFunc: func(_ context.Context, _, _ string) error { removed = true; return nil },
			},
			nil, nil, nil, developerRoles(),
		)

		err := svc.RemoveTestCaseEnvironment(context.Background(), testutil.MakeUser(), "product-001", "tc-001", "env-001")
		if err != nil {
			t.Fatalf("RemoveTestCaseEnvironment: %v", err)
		}
		if !removed {
			t.Fatal("environment was not removed")
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newTestingService(nil, nil, nil, nil, forbiddenRoles())
		err := svc.RemoveTestCaseEnvironment(context.Background(), testutil.MakeUser(), "product-001", "tc-001", "env-001")
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

func TestListTestCaseEnvironments(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		svc := newTestingService(
			&testutil.MockTestCaseStore{
				ListEnvironmentsFunc: func(_ context.Context, _ string) ([]string, error) {
					return []string{"env-001"}, nil
				},
			},
			nil, nil, nil, viewerRoles(),
		)

		result, err := svc.ListTestCaseEnvironments(context.Background(), testutil.MakeUser(), "product-001", "tc-001")
		if err != nil {
			t.Fatalf("ListTestCaseEnvironments: %v", err)
		}
		if len(result) != 1 {
			t.Fatalf("len(result) = %d, want 1", len(result))
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newTestingService(nil, nil, nil, nil, forbiddenRoles())
		_, err := svc.ListTestCaseEnvironments(context.Background(), testutil.MakeUser(), "product-001", "tc-001")
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

// --- Campaigns ---

// scopedTestCases returns a test case store where every requested test case
// belongs to product-001 and carries the given environments.
func scopedTestCases(envs map[string][]string) *testutil.MockTestCaseStore {
	return &testutil.MockTestCaseStore{
		GetByIDFunc: func(_ context.Context, id string) (*model.TestCase, error) {
			return testutil.MakeTestCase(func(tc *model.TestCase) { tc.ID = id }), nil
		},
		ListEnvironmentsFunc: func(_ context.Context, testCaseID string) ([]string, error) {
			return envs[testCaseID], nil
		},
	}
}

func TestCreateCampaign(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		var created *model.TestingCampaign
		var seeded []*model.CampaignResult
		svc := newTestingService(
			scopedTestCases(map[string][]string{"tc-001": {"env-001", "env-002"}}),
			&testutil.MockTestingCampaignStore{
				CreateFunc: func(_ context.Context, c *model.TestingCampaign) error { created = c; return nil },
			},
			&testutil.MockCampaignResultStore{
				CreateBatchFunc: func(_ context.Context, rs []*model.CampaignResult) error { seeded = rs; return nil },
			},
			noopAudit(), ownerRoles(),
		)

		result, err := svc.CreateCampaign(context.Background(), testutil.MakeUser(), CreateCampaignInput{
			ProductID: "product-001", Name: "Release 1.0", TestCaseIDs: []string{"tc-001"},
		})
		if err != nil {
			t.Fatalf("CreateCampaign: %v", err)
		}
		if result.Name != "Release 1.0" {
			t.Fatalf("Name = %q, want %q", result.Name, "Release 1.0")
		}
		if result.Status != model.CampaignStatusDraft {
			t.Fatalf("Status = %q, want %q", result.Status, model.CampaignStatusDraft)
		}
		if created == nil {
			t.Fatal("campaign was not created")
		}
		// One result per linked environment (FR-27).
		if len(seeded) != 2 {
			t.Fatalf("seeded %d results, want 2", len(seeded))
		}
		for _, r := range seeded {
			if r.Status != model.CampaignResultStatusPending {
				t.Fatalf("seeded status = %q, want pending", r.Status)
			}
			if r.EnvironmentID == "" {
				t.Fatal("seeded result lost its environment")
			}
		}
	})

	t.Run("test case without environments seeds one result", func(t *testing.T) {
		var seeded []*model.CampaignResult
		svc := newTestingService(
			scopedTestCases(map[string][]string{"tc-001": nil}),
			&testutil.MockTestingCampaignStore{
				CreateFunc: func(_ context.Context, _ *model.TestingCampaign) error { return nil },
			},
			&testutil.MockCampaignResultStore{
				CreateBatchFunc: func(_ context.Context, rs []*model.CampaignResult) error { seeded = rs; return nil },
			},
			noopAudit(), ownerRoles(),
		)

		if _, err := svc.CreateCampaign(context.Background(), testutil.MakeUser(), CreateCampaignInput{
			ProductID: "product-001", Name: "Release 1.0", TestCaseIDs: []string{"tc-001"},
		}); err != nil {
			t.Fatalf("CreateCampaign: %v", err)
		}
		if len(seeded) != 1 {
			t.Fatalf("seeded %d results, want 1", len(seeded))
		}
		if seeded[0].EnvironmentID != "" {
			t.Fatalf("EnvironmentID = %q, want empty", seeded[0].EnvironmentID)
		}
	})

	t.Run("rejects test case from another product", func(t *testing.T) {
		svc := newTestingService(
			&testutil.MockTestCaseStore{
				GetByIDFunc: func(_ context.Context, id string) (*model.TestCase, error) {
					return testutil.MakeTestCase(func(tc *model.TestCase) {
						tc.ID = id
						tc.ProductID = "product-999"
					}), nil
				},
			},
			&testutil.MockTestingCampaignStore{}, &testutil.MockCampaignResultStore{},
			noopAudit(), ownerRoles(),
		)

		_, err := svc.CreateCampaign(context.Background(), testutil.MakeUser(), CreateCampaignInput{
			ProductID: "product-001", Name: "X", TestCaseIDs: []string{"tc-001"},
		})
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})

	t.Run("missing name", func(t *testing.T) {
		svc := newTestingService(nil, nil, nil, nil, ownerRoles())
		_, err := svc.CreateCampaign(context.Background(), testutil.MakeUser(), CreateCampaignInput{ProductID: "product-001"})
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("empty scope", func(t *testing.T) {
		svc := newTestingService(nil, nil, nil, nil, ownerRoles())
		_, err := svc.CreateCampaign(context.Background(), testutil.MakeUser(), CreateCampaignInput{
			ProductID: "product-001", Name: "X",
		})
		if err == nil {
			t.Fatal("expected error for a campaign with no test cases")
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newTestingService(nil, nil, nil, nil, forbiddenRoles())
		_, err := svc.CreateCampaign(context.Background(), testutil.MakeUser(), CreateCampaignInput{
			ProductID: "product-001", Name: "X", TestCaseIDs: []string{"tc-001"},
		})
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

func TestActivateCampaign(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		var updated *model.TestingCampaign
		svc := newTestingService(nil,
			&testutil.MockTestingCampaignStore{
				GetByIDFunc: func(_ context.Context, _ string) (*model.TestingCampaign, error) {
					return testutil.MakeTestingCampaign(), nil
				},
				UpdateFunc: func(_ context.Context, c *model.TestingCampaign) error { updated = c; return nil },
			},
			nil, noopAudit(), ownerRoles(),
		)

		result, err := svc.ActivateCampaign(context.Background(), testutil.MakeUser(), "product-001", "campaign-001")
		if err != nil {
			t.Fatalf("ActivateCampaign: %v", err)
		}
		if result.Status != model.CampaignStatusActive {
			t.Fatalf("Status = %q, want %q", result.Status, model.CampaignStatusActive)
		}
		if updated == nil {
			t.Fatal("campaign was not persisted")
		}
	})

	t.Run("rejects a second active campaign", func(t *testing.T) {
		svc := newTestingService(nil,
			&testutil.MockTestingCampaignStore{
				GetByIDFunc: func(_ context.Context, _ string) (*model.TestingCampaign, error) {
					return testutil.MakeTestingCampaign(), nil
				},
				GetActiveByProductIDFunc: func(_ context.Context, _ string) (*model.TestingCampaign, error) {
					return testutil.MakeTestingCampaign(func(c *model.TestingCampaign) {
						c.ID = "campaign-002"
						c.Status = model.CampaignStatusActive
					}), nil
				},
			},
			nil, noopAudit(), ownerRoles(),
		)

		if _, err := svc.ActivateCampaign(context.Background(), testutil.MakeUser(), "product-001", "campaign-001"); err == nil {
			t.Fatal("expected error when another campaign is already active")
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newTestingService(nil, nil, nil, nil, testerRoles())
		_, err := svc.ActivateCampaign(context.Background(), testutil.MakeUser(), "product-001", "campaign-001")
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

func TestCloseCampaign(t *testing.T) {
	activeStore := func(updated **model.TestingCampaign) *testutil.MockTestingCampaignStore {
		return &testutil.MockTestingCampaignStore{
			GetByIDFunc: func(_ context.Context, _ string) (*model.TestingCampaign, error) {
				return testutil.MakeTestingCampaign(func(c *model.TestingCampaign) {
					c.Status = model.CampaignStatusActive
				}), nil
			},
			UpdateFunc: func(_ context.Context, c *model.TestingCampaign) error { *updated = c; return nil },
		}
	}

	t.Run("success records who closed it", func(t *testing.T) {
		var updated *model.TestingCampaign
		var entries []*model.AuditEntry
		svc := newTestingService(nil, activeStore(&updated),
			&testutil.MockCampaignResultStore{
				ListByCampaignIDFunc: func(_ context.Context, _ string) ([]*model.CampaignResult, error) {
					return []*model.CampaignResult{
						testutil.MakeCampaignResult(func(cr *model.CampaignResult) {
							cr.Status = model.CampaignResultStatusPassed
						}),
					}, nil
				},
			},
			&testutil.MockAuditStore{
				CreateFunc: func(_ context.Context, e *model.AuditEntry) error {
					entries = append(entries, e)
					return nil
				},
			},
			ownerRoles(),
		)

		user := testutil.MakeUser()
		result, err := svc.CloseCampaign(context.Background(), user, "product-001", "campaign-001", false)
		if err != nil {
			t.Fatalf("CloseCampaign: %v", err)
		}
		if result.Status != model.CampaignStatusClosed {
			t.Fatalf("Status = %q, want %q", result.Status, model.CampaignStatusClosed)
		}
		if result.ClosedBy != user.ID {
			t.Fatalf("ClosedBy = %q, want %q", result.ClosedBy, user.ID)
		}
		if updated == nil {
			t.Fatal("campaign was not persisted")
		}
		// The audit entry must name the transition (FR-106).
		if len(entries) != 1 {
			t.Fatalf("recorded %d audit entries, want 1", len(entries))
		}
		if entries[0].FieldName != "status" ||
			entries[0].OldValue != model.CampaignStatusActive ||
			entries[0].NewValue != model.CampaignStatusClosed {
			t.Fatalf("audit entry = %+v, want a status active->closed change", entries[0])
		}
	})

	t.Run("incomplete results need confirmation", func(t *testing.T) {
		var updated *model.TestingCampaign
		svc := newTestingService(nil, activeStore(&updated),
			&testutil.MockCampaignResultStore{
				ListByCampaignIDFunc: func(_ context.Context, _ string) ([]*model.CampaignResult, error) {
					return []*model.CampaignResult{
						testutil.MakeCampaignResult(),
						testutil.MakeCampaignResult(func(cr *model.CampaignResult) {
							cr.ID = "cr-002"
							cr.Status = model.CampaignResultStatusBlocked
						}),
						testutil.MakeCampaignResult(func(cr *model.CampaignResult) {
							cr.ID = "cr-003"
							cr.Status = model.CampaignResultStatusSkipped
						}),
					}, nil
				},
			},
			noopAudit(), ownerRoles(),
		)

		_, err := svc.CloseCampaign(context.Background(), testutil.MakeUser(), "product-001", "campaign-001", false)
		var incomplete *IncompleteResultsError
		if !errors.As(err, &incomplete) {
			t.Fatalf("err = %v, want *IncompleteResultsError", err)
		}
		// Pending and blocked count; skipped does not (FR-47).
		if incomplete.Incomplete != 2 {
			t.Fatalf("Incomplete = %d, want 2", incomplete.Incomplete)
		}
		if updated != nil {
			t.Fatal("campaign must not be closed without confirmation")
		}
	})

	t.Run("force closes anyway", func(t *testing.T) {
		var updated *model.TestingCampaign
		svc := newTestingService(nil, activeStore(&updated), nil, noopAudit(), ownerRoles())

		if _, err := svc.CloseCampaign(context.Background(), testutil.MakeUser(), "product-001", "campaign-001", true); err != nil {
			t.Fatalf("CloseCampaign: %v", err)
		}
		if updated == nil {
			t.Fatal("campaign was not persisted")
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newTestingService(nil, nil, nil, nil, testerRoles())
		_, err := svc.CloseCampaign(context.Background(), testutil.MakeUser(), "product-001", "campaign-001", true)
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

func TestGetCampaign(t *testing.T) {
	campaign := testutil.MakeTestingCampaign()

	t.Run("success", func(t *testing.T) {
		svc := newTestingService(nil,
			&testutil.MockTestingCampaignStore{
				GetByIDFunc: func(_ context.Context, _ string) (*model.TestingCampaign, error) { return campaign, nil },
			},
			nil, nil, viewerRoles(),
		)

		result, err := svc.GetCampaign(context.Background(), testutil.MakeUser(), "product-001", "campaign-001")
		if err != nil {
			t.Fatalf("GetCampaign: %v", err)
		}
		if result.ID != campaign.ID {
			t.Fatalf("ID = %q, want %q", result.ID, campaign.ID)
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newTestingService(nil, nil, nil, nil, forbiddenRoles())
		_, err := svc.GetCampaign(context.Background(), testutil.MakeUser(), "product-001", "campaign-001")
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

func TestUpdateCampaign(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		campaign := testutil.MakeTestingCampaign()
		var updated *model.TestingCampaign
		svc := newTestingService(nil,
			&testutil.MockTestingCampaignStore{
				GetByIDFunc: func(_ context.Context, _ string) (*model.TestingCampaign, error) { return campaign, nil },
				UpdateFunc:  func(_ context.Context, c *model.TestingCampaign) error { updated = c; return nil },
			},
			nil, noopAudit(), ownerRoles(),
		)

		newName := "Updated"
		result, err := svc.UpdateCampaign(context.Background(), testutil.MakeUser(), "product-001", "campaign-001", UpdateCampaignInput{Name: &newName})
		if err != nil {
			t.Fatalf("UpdateCampaign: %v", err)
		}
		if result.Name != "Updated" {
			t.Fatalf("Name = %q, want %q", result.Name, "Updated")
		}
		if updated == nil {
			t.Fatal("campaign was not updated")
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newTestingService(nil, nil, nil, nil, forbiddenRoles())
		_, err := svc.UpdateCampaign(context.Background(), testutil.MakeUser(), "product-001", "campaign-001", UpdateCampaignInput{})
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

func TestDeleteCampaign(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		deleted := false
		svc := newTestingService(nil,
			&testutil.MockTestingCampaignStore{
				DeleteFunc: func(_ context.Context, _ string) error { deleted = true; return nil },
			},
			nil, noopAudit(), ownerRoles(),
		)

		err := svc.DeleteCampaign(context.Background(), testutil.MakeUser(), "product-001", "campaign-001")
		if err != nil {
			t.Fatalf("DeleteCampaign: %v", err)
		}
		if !deleted {
			t.Fatal("campaign was not deleted")
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newTestingService(nil, nil, nil, nil, forbiddenRoles())
		err := svc.DeleteCampaign(context.Background(), testutil.MakeUser(), "product-001", "campaign-001")
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

func TestListCampaigns(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		svc := newTestingService(nil,
			&testutil.MockTestingCampaignStore{
				ListByProductIDFunc: func(_ context.Context, _ string) ([]*model.TestingCampaign, error) {
					return []*model.TestingCampaign{testutil.MakeTestingCampaign()}, nil
				},
			},
			nil, nil, viewerRoles(),
		)

		result, err := svc.ListCampaigns(context.Background(), testutil.MakeUser(), "product-001")
		if err != nil {
			t.Fatalf("ListCampaigns: %v", err)
		}
		if len(result) != 1 {
			t.Fatalf("len(result) = %d, want 1", len(result))
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newTestingService(nil, nil, nil, nil, forbiddenRoles())
		_, err := svc.ListCampaigns(context.Background(), testutil.MakeUser(), "product-001")
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

func TestListCampaignResults(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		svc := newTestingService(nil, nil,
			&testutil.MockCampaignResultStore{
				ListByCampaignIDFunc: func(_ context.Context, _ string) ([]*model.CampaignResult, error) {
					return []*model.CampaignResult{{ID: "cr-001", Status: model.CampaignResultStatusPending}}, nil
				},
			},
			nil, viewerRoles(),
		)

		result, err := svc.ListCampaignResults(context.Background(), testutil.MakeUser(), "product-001", "campaign-001")
		if err != nil {
			t.Fatalf("ListCampaignResults: %v", err)
		}
		if len(result) != 1 {
			t.Fatalf("len(result) = %d, want 1", len(result))
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newTestingService(nil, nil, nil, nil, forbiddenRoles())
		_, err := svc.ListCampaignResults(context.Background(), testutil.MakeUser(), "product-001", "campaign-001")
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

func TestSetResultStatus(t *testing.T) {
	pendingResult := func(updated **model.CampaignResult) *testutil.MockCampaignResultStore {
		return &testutil.MockCampaignResultStore{
			GetByIDFunc: func(_ context.Context, id string) (*model.CampaignResult, error) {
				return testutil.MakeCampaignResult(func(cr *model.CampaignResult) { cr.ID = id }), nil
			},
			UpdateFunc: func(_ context.Context, cr *model.CampaignResult) error { *updated = cr; return nil },
		}
	}

	t.Run("success records the tester and an event", func(t *testing.T) {
		var updated *model.CampaignResult
		var events []*model.CampaignResultEvent
		svc := newTestingService(nil, activeCampaign(), pendingResult(&updated), nil, testerRoles())
		svc.campaignEvents = &testutil.MockCampaignResultEventStore{
			CreateFunc: func(_ context.Context, ev *model.CampaignResultEvent) error {
				events = append(events, ev)
				return nil
			},
		}

		user := testutil.MakeUser()
		result, err := svc.SetResultStatus(context.Background(), user,
			"product-001", "campaign-001", "cr-001", model.CampaignResultStatusPassed, "")
		if err != nil {
			t.Fatalf("SetResultStatus: %v", err)
		}
		if result.Status != model.CampaignResultStatusPassed {
			t.Fatalf("Status = %q, want %q", result.Status, model.CampaignResultStatusPassed)
		}
		if result.ExecutedBy != user.ID || result.ExecutedAt == "" {
			t.Fatalf("execution not recorded: by=%q at=%q", result.ExecutedBy, result.ExecutedAt)
		}
		if updated == nil {
			t.Fatal("result was not persisted")
		}
		if len(events) != 1 {
			t.Fatalf("recorded %d events, want 1", len(events))
		}
		if events[0].FromStatus != model.CampaignResultStatusPending ||
			events[0].ToStatus != model.CampaignResultStatusPassed {
			t.Fatalf("event = %+v, want pending->passed", events[0])
		}
	})

	t.Run("failing requires a defect", func(t *testing.T) {
		var updated *model.CampaignResult
		svc := newTestingService(nil, activeCampaign(), pendingResult(&updated), nil, testerRoles())

		_, err := svc.SetResultStatus(context.Background(), testutil.MakeUser(),
			"product-001", "campaign-001", "cr-001", model.CampaignResultStatusFailed, "")
		if err == nil {
			t.Fatal("expected error when failing without a defect")
		}
		if updated != nil {
			t.Fatal("result must not change without a defect")
		}
	})

	t.Run("failing with a defect records it", func(t *testing.T) {
		var updated *model.CampaignResult
		svc := newTestingService(nil, activeCampaign(), pendingResult(&updated), nil, testerRoles())

		result, err := svc.SetResultStatus(context.Background(), testutil.MakeUser(),
			"product-001", "campaign-001", "cr-001", model.CampaignResultStatusFailed, "defect-001")
		if err != nil {
			t.Fatalf("SetResultStatus: %v", err)
		}
		if result.DefectID != "defect-001" {
			t.Fatalf("DefectID = %q, want defect-001", result.DefectID)
		}
	})

	t.Run("passing clears a previous defect", func(t *testing.T) {
		svc := newTestingService(nil, activeCampaign(),
			&testutil.MockCampaignResultStore{
				GetByIDFunc: func(_ context.Context, id string) (*model.CampaignResult, error) {
					return testutil.MakeCampaignResult(func(cr *model.CampaignResult) {
						cr.ID = id
						cr.Status = model.CampaignResultStatusRetest
						cr.DefectID = "defect-001"
					}), nil
				},
				UpdateFunc: func(_ context.Context, _ *model.CampaignResult) error { return nil },
			},
			nil, testerRoles(),
		)

		result, err := svc.SetResultStatus(context.Background(), testutil.MakeUser(),
			"product-001", "campaign-001", "cr-001", model.CampaignResultStatusPassed, "")
		if err != nil {
			t.Fatalf("SetResultStatus: %v", err)
		}
		if result.DefectID != "" {
			t.Fatalf("DefectID = %q, want empty", result.DefectID)
		}
	})

	t.Run("rejects an unknown status", func(t *testing.T) {
		svc := newTestingService(nil, activeCampaign(), nil, nil, testerRoles())
		_, err := svc.SetResultStatus(context.Background(), testutil.MakeUser(),
			"product-001", "campaign-001", "cr-001", "nonsense", "")
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("rejects changes once the campaign is closed", func(t *testing.T) {
		var updated *model.CampaignResult
		svc := newTestingService(nil,
			&testutil.MockTestingCampaignStore{
				GetByIDFunc: func(_ context.Context, _ string) (*model.TestingCampaign, error) {
					return testutil.MakeTestingCampaign(func(c *model.TestingCampaign) {
						c.Status = model.CampaignStatusClosed
					}), nil
				},
			},
			pendingResult(&updated), nil, testerRoles(),
		)

		_, err := svc.SetResultStatus(context.Background(), testutil.MakeUser(),
			"product-001", "campaign-001", "cr-001", model.CampaignResultStatusPassed, "")
		if err == nil {
			t.Fatal("expected error for a closed campaign")
		}
		if updated != nil {
			t.Fatal("result of a closed campaign must not change")
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newTestingService(nil, nil, nil, nil, forbiddenRoles())
		_, err := svc.SetResultStatus(context.Background(), testutil.MakeUser(),
			"product-001", "campaign-001", "cr-001", model.CampaignResultStatusPassed, "")
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

func TestMarkResultsForRetest(t *testing.T) {
	t.Run("only failed results move to retest", func(t *testing.T) {
		var updated []*model.CampaignResult
		svc := newTestingService(nil, activeCampaign(),
			&testutil.MockCampaignResultStore{
				ListByDefectIDFunc: func(_ context.Context, _ string) ([]*model.CampaignResult, error) {
					return []*model.CampaignResult{
						testutil.MakeCampaignResult(func(cr *model.CampaignResult) {
							cr.Status = model.CampaignResultStatusFailed
							cr.DefectID = "defect-001"
						}),
						testutil.MakeCampaignResult(func(cr *model.CampaignResult) {
							cr.ID = "cr-002"
							cr.Status = model.CampaignResultStatusSkipped
							cr.DefectID = "defect-001"
						}),
					}, nil
				},
				UpdateFunc: func(_ context.Context, cr *model.CampaignResult) error {
					updated = append(updated, cr)
					return nil
				},
			},
			nil, ownerRoles(),
		)

		if err := svc.MarkResultsForRetest(context.Background(), "defect-001", "user-001"); err != nil {
			t.Fatalf("MarkResultsForRetest: %v", err)
		}
		if len(updated) != 1 {
			t.Fatalf("updated %d results, want 1", len(updated))
		}
		// Never straight to passed — a tester must confirm (FR-123).
		if updated[0].Status != model.CampaignResultStatusRetest {
			t.Fatalf("Status = %q, want %q", updated[0].Status, model.CampaignResultStatusRetest)
		}
	})

	t.Run("results of a closed campaign are left alone", func(t *testing.T) {
		// A closed campaign records the verdict it was closed with. Fixing a
		// defect afterwards must not reopen one of its results, which would
		// retroactively turn a decided campaign back into an incomplete one.
		var updated []*model.CampaignResult
		svc := newTestingService(nil,
			&testutil.MockTestingCampaignStore{
				GetByIDFunc: func(_ context.Context, id string) (*model.TestingCampaign, error) {
					return testutil.MakeTestingCampaign(func(c *model.TestingCampaign) {
						c.ID = id
						c.Status = model.CampaignStatusClosed
					}), nil
				},
			},
			&testutil.MockCampaignResultStore{
				ListByDefectIDFunc: func(_ context.Context, _ string) ([]*model.CampaignResult, error) {
					return []*model.CampaignResult{
						testutil.MakeCampaignResult(func(cr *model.CampaignResult) {
							cr.Status = model.CampaignResultStatusFailed
							cr.DefectID = "defect-001"
						}),
					}, nil
				},
				UpdateFunc: func(_ context.Context, cr *model.CampaignResult) error {
					updated = append(updated, cr)
					return nil
				},
			},
			nil, ownerRoles(),
		)

		if err := svc.MarkResultsForRetest(context.Background(), "defect-001", "user-001"); err != nil {
			t.Fatalf("MarkResultsForRetest: %v", err)
		}
		if len(updated) != 0 {
			t.Fatalf("updated %d results of a closed campaign, want 0", len(updated))
		}
	})

	t.Run("each campaign is looked up once", func(t *testing.T) {
		// Two results in the same campaign must not cost two store reads.
		lookups := 0
		var updated []*model.CampaignResult
		svc := newTestingService(nil,
			&testutil.MockTestingCampaignStore{
				GetByIDFunc: func(_ context.Context, id string) (*model.TestingCampaign, error) {
					lookups++
					return testutil.MakeTestingCampaign(func(c *model.TestingCampaign) {
						c.ID = id
						c.Status = model.CampaignStatusActive
					}), nil
				},
			},
			&testutil.MockCampaignResultStore{
				ListByDefectIDFunc: func(_ context.Context, _ string) ([]*model.CampaignResult, error) {
					return []*model.CampaignResult{
						testutil.MakeCampaignResult(func(cr *model.CampaignResult) {
							cr.Status = model.CampaignResultStatusFailed
							cr.DefectID = "defect-001"
						}),
						testutil.MakeCampaignResult(func(cr *model.CampaignResult) {
							cr.ID = "cr-002"
							cr.Status = model.CampaignResultStatusFailed
							cr.DefectID = "defect-001"
						}),
					}, nil
				},
				UpdateFunc: func(_ context.Context, cr *model.CampaignResult) error {
					updated = append(updated, cr)
					return nil
				},
			},
			nil, ownerRoles(),
		)

		if err := svc.MarkResultsForRetest(context.Background(), "defect-001", "user-001"); err != nil {
			t.Fatalf("MarkResultsForRetest: %v", err)
		}
		if len(updated) != 2 {
			t.Fatalf("updated %d results, want 2", len(updated))
		}
		if lookups != 1 {
			t.Fatalf("campaign looked up %d times, want 1", lookups)
		}
	})
}

// --- Result event reads ---

func TestListEventsForResult(t *testing.T) {
	campaignResults := func() *testutil.MockCampaignResultStore {
		return &testutil.MockCampaignResultStore{
			GetByIDFunc: func(_ context.Context, id string) (*model.CampaignResult, error) {
				return testutil.MakeCampaignResult(func(cr *model.CampaignResult) { cr.ID = id }), nil
			},
		}
	}

	t.Run("success", func(t *testing.T) {
		svc := newTestingService(nil, activeCampaign(), campaignResults(), nil, viewerRoles())
		svc.campaignEvents = &testutil.MockCampaignResultEventStore{
			ListByResultIDFunc: func(_ context.Context, resultID string) ([]*model.CampaignResultEvent, error) {
				return []*model.CampaignResultEvent{
					testutil.MakeCampaignResultEvent(func(ev *model.CampaignResultEvent) { ev.ResultID = resultID }),
				}, nil
			},
		}

		events, err := svc.ListEventsForResult(context.Background(), testutil.MakeUser(),
			"product-001", "campaign-001", "cr-001")
		if err != nil {
			t.Fatalf("ListEventsForResult: %v", err)
		}
		if len(events) != 1 {
			t.Fatalf("len(events) = %d, want 1", len(events))
		}
	})

	t.Run("result from another campaign is not found", func(t *testing.T) {
		svc := newTestingService(nil, activeCampaign(), &testutil.MockCampaignResultStore{
			GetByIDFunc: func(_ context.Context, id string) (*model.CampaignResult, error) {
				return testutil.MakeCampaignResult(func(cr *model.CampaignResult) {
					cr.ID = id
					cr.CampaignID = "campaign-999"
				}), nil
			},
		}, nil, viewerRoles())

		_, err := svc.ListEventsForResult(context.Background(), testutil.MakeUser(),
			"product-001", "campaign-001", "cr-001")
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("err = %v, want ErrNotFound", err)
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newTestingService(nil, nil, nil, nil, forbiddenRoles())
		_, err := svc.ListEventsForResult(context.Background(), testutil.MakeUser(),
			"product-001", "campaign-001", "cr-001")
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

func TestResultChurn(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		svc := newTestingService(nil, nil, nil, nil, viewerRoles())
		svc.campaignEvents = &testutil.MockCampaignResultEventStore{
			CountsByCampaignIDFunc: func(_ context.Context, _ string) (map[string]int, error) {
				return map[string]int{"cr-001": 3}, nil
			},
		}

		counts, err := svc.ResultChurn(context.Background(), testutil.MakeUser(), "product-001", "campaign-001")
		if err != nil {
			t.Fatalf("ResultChurn: %v", err)
		}
		if counts["cr-001"] != 3 {
			t.Fatalf("counts[cr-001] = %d, want 3", counts["cr-001"])
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newTestingService(nil, nil, nil, nil, forbiddenRoles())
		_, err := svc.ResultChurn(context.Background(), testutil.MakeUser(), "product-001", "campaign-001")
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

func TestListVerificationEvents(t *testing.T) {
	productCampaigns := func() *testutil.MockTestingCampaignStore {
		return &testutil.MockTestingCampaignStore{
			ListByProductIDFunc: func(_ context.Context, productID string) ([]*model.TestingCampaign, error) {
				return []*model.TestingCampaign{
					testutil.MakeTestingCampaign(func(c *model.TestingCampaign) {
						c.ID = "campaign-001"
						c.ProductID = productID
					}),
				}, nil
			},
		}
	}

	t.Run("success returns the trail including the pass that clears the defect", func(t *testing.T) {
		svc := newTestingService(nil, productCampaigns(), nil, nil, viewerRoles())
		svc.campaignEvents = &testutil.MockCampaignResultEventStore{
			ListByDefectTrailFunc: func(_ context.Context, _ string) ([]*model.CampaignResultEvent, error) {
				return []*model.CampaignResultEvent{
					testutil.MakeCampaignResultEvent(func(ev *model.CampaignResultEvent) {
						ev.ToStatus = model.CampaignResultStatusFailed
						ev.DefectID = "defect-001"
					}),
					testutil.MakeCampaignResultEvent(func(ev *model.CampaignResultEvent) {
						ev.ToStatus = model.CampaignResultStatusPassed
						ev.DefectID = ""
					}),
				}, nil
			},
		}

		events, err := svc.ListVerificationEvents(context.Background(), testutil.MakeUser(),
			"product-001", "defect-001")
		if err != nil {
			t.Fatalf("ListVerificationEvents: %v", err)
		}
		if len(events) != 2 {
			t.Fatalf("len(events) = %d, want 2", len(events))
		}
		if events[1].ToStatus != model.CampaignResultStatusPassed || events[1].DefectID != "" {
			t.Fatalf("closing event = %+v, want a pass carrying no defect", events[1])
		}
	})

	t.Run("events from another product are dropped", func(t *testing.T) {
		svc := newTestingService(nil, productCampaigns(), nil, nil, viewerRoles())
		svc.campaignEvents = &testutil.MockCampaignResultEventStore{
			ListByDefectTrailFunc: func(_ context.Context, _ string) ([]*model.CampaignResultEvent, error) {
				return []*model.CampaignResultEvent{
					testutil.MakeCampaignResultEvent(func(ev *model.CampaignResultEvent) {
						ev.CampaignID = "campaign-of-another-product"
					}),
				}, nil
			},
		}

		events, err := svc.ListVerificationEvents(context.Background(), testutil.MakeUser(),
			"product-001", "defect-001")
		if err != nil {
			t.Fatalf("ListVerificationEvents: %v", err)
		}
		if len(events) != 0 {
			t.Fatalf("len(events) = %d, want 0", len(events))
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newTestingService(nil, nil, nil, nil, forbiddenRoles())
		_, err := svc.ListVerificationEvents(context.Background(), testutil.MakeUser(),
			"product-001", "defect-001")
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}
