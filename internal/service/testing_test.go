package service

import (
	"context"
	"errors"
	"testing"

	"nexmap.net/internal/model"
	"nexmap.net/internal/testutil"
)

func newTestingService(
	testCases *testutil.MockTestCaseStore,
	campaigns *testutil.MockTestingCampaignStore,
	campaignResults *testutil.MockCampaignResultStore,
	audit *testutil.MockAuditStore,
	roles *testutil.MockProductRoleStore,
) *TestingService {
	return &TestingService{
		testCases:       testCases,
		campaigns:       campaigns,
		campaignResults: campaignResults,
		comments:        &recordingPurger{},
		audit:           audit,
		authz:           &Authorizer{productRoles: roles},
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

func TestCreateCampaign(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		var created *model.TestingCampaign
		svc := newTestingService(nil,
			&testutil.MockTestingCampaignStore{
				CreateFunc: func(_ context.Context, c *model.TestingCampaign) error { created = c; return nil },
			},
			nil, noopAudit(), ownerRoles(),
		)

		result, err := svc.CreateCampaign(context.Background(), testutil.MakeUser(), CreateCampaignInput{
			ProductID: "product-001", Name: "Release 1.0",
		})
		if err != nil {
			t.Fatalf("CreateCampaign: %v", err)
		}
		if result.Name != "Release 1.0" {
			t.Fatalf("Name = %q, want %q", result.Name, "Release 1.0")
		}
		if created == nil {
			t.Fatal("campaign was not created")
		}
	})

	t.Run("missing name", func(t *testing.T) {
		svc := newTestingService(nil, nil, nil, nil, ownerRoles())
		_, err := svc.CreateCampaign(context.Background(), testutil.MakeUser(), CreateCampaignInput{ProductID: "product-001"})
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newTestingService(nil, nil, nil, nil, forbiddenRoles())
		_, err := svc.CreateCampaign(context.Background(), testutil.MakeUser(), CreateCampaignInput{ProductID: "product-001", Name: "X"})
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

func TestUpdateCampaignResult(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		var updated *model.CampaignResult
		svc := newTestingService(nil, nil,
			&testutil.MockCampaignResultStore{
				GetByCampaignAndTestCaseFunc: func(_ context.Context, _, _, _ string) (*model.CampaignResult, error) {
					return &model.CampaignResult{
						ID:         "cr-001",
						CampaignID: "campaign-001",
						TestCaseID: "tc-001",
						Status:     model.CampaignResultStatusPending,
					}, nil
				},
				UpdateFunc: func(_ context.Context, cr *model.CampaignResult) error { updated = cr; return nil },
			},
			nil, testerRoles(),
		)

		err := svc.UpdateCampaignResult(context.Background(), testutil.MakeUser(), "product-001", "campaign-001", "tc-001", "", model.CampaignResultStatusPassed)
		if err != nil {
			t.Fatalf("UpdateCampaignResult: %v", err)
		}
		if updated == nil {
			t.Fatal("result was not updated")
		}
		if updated.Status != model.CampaignResultStatusPassed {
			t.Fatalf("Status = %q, want %q", updated.Status, model.CampaignResultStatusPassed)
		}
	})

	t.Run("result not found", func(t *testing.T) {
		svc := newTestingService(nil, nil,
			&testutil.MockCampaignResultStore{
				GetByCampaignAndTestCaseFunc: func(_ context.Context, _, _, _ string) (*model.CampaignResult, error) {
					return nil, errors.New("not found")
				},
			},
			nil, testerRoles(),
		)

		err := svc.UpdateCampaignResult(context.Background(), testutil.MakeUser(), "product-001", "campaign-001", "tc-001", "", model.CampaignResultStatusPassed)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newTestingService(nil, nil, nil, nil, forbiddenRoles())
		err := svc.UpdateCampaignResult(context.Background(), testutil.MakeUser(), "product-001", "campaign-001", "tc-001", "", model.CampaignResultStatusPassed)
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}
