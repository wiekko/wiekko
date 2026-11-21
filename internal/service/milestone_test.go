// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package service

import (
	"context"
	"errors"
	"testing"

	"nexmap.net/internal/model"
	"nexmap.net/internal/testutil"
)

func newMilestoneService(
	milestones *testutil.MockMilestoneStore,
	sprints *testutil.MockSprintStore,
	statuses *testutil.MockStatusStore,
	products *testutil.MockProductStore,
	pyramidLevels *testutil.MockPyramidLevelStore,
	runtimeEnvs *testutil.MockRuntimeEnvironmentStore,
	audit *testutil.MockAuditStore,
	roles *testutil.MockProductRoleStore,
) *MilestoneService {
	return &MilestoneService{
		owns:          testOwnership(),
		milestones:    milestones,
		sprints:       sprints,
		statuses:      statuses,
		products:      products,
		pyramidLevels: pyramidLevels,
		runtimeEnvs:   runtimeEnvs,
		comments:      &recordingPurger{},
		audit:         audit,
		authz:         &Authorizer{productRoles: roles},
	}
}

// --- Milestones ---

func TestCreateMilestone(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		var created *model.Milestone
		svc := newMilestoneService(
			&testutil.MockMilestoneStore{
				CreateFunc: func(_ context.Context, m *model.Milestone) error { created = m; return nil },
			},
			nil, nil, nil, nil, nil, noopAudit(), ownerRoles(),
		)

		result, err := svc.CreateMilestone(context.Background(), testutil.MakeUser(), CreateMilestoneInput{
			ProductID: "product-001", Name: "v1.0",
		})
		if err != nil {
			t.Fatalf("CreateMilestone: %v", err)
		}
		if result.Name != "v1.0" {
			t.Fatalf("Name = %q, want %q", result.Name, "v1.0")
		}
		if created == nil {
			t.Fatal("milestone was not created")
		}
		if created.Type != model.MilestoneTypePlanning {
			t.Fatalf("Type = %q, want %q", created.Type, model.MilestoneTypePlanning)
		}
	})

	t.Run("missing name", func(t *testing.T) {
		svc := newMilestoneService(nil, nil, nil, nil, nil, nil, nil, ownerRoles())
		_, err := svc.CreateMilestone(context.Background(), testutil.MakeUser(), CreateMilestoneInput{ProductID: "product-001"})
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newMilestoneService(nil, nil, nil, nil, nil, nil, nil, forbiddenRoles())
		_, err := svc.CreateMilestone(context.Background(), testutil.MakeUser(), CreateMilestoneInput{ProductID: "product-001", Name: "X"})
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

func TestGetMilestone(t *testing.T) {
	milestone := testutil.MakeMilestone()

	t.Run("success", func(t *testing.T) {
		svc := newMilestoneService(
			&testutil.MockMilestoneStore{
				GetByIDFunc: func(_ context.Context, _ string) (*model.Milestone, error) { return milestone, nil },
			},
			nil, nil, nil, nil, nil, nil, viewerRoles(),
		)

		result, err := svc.GetMilestone(context.Background(), testutil.MakeUser(), "product-001", "milestone-001")
		if err != nil {
			t.Fatalf("GetMilestone: %v", err)
		}
		if result.ID != milestone.ID {
			t.Fatalf("ID = %q, want %q", result.ID, milestone.ID)
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newMilestoneService(nil, nil, nil, nil, nil, nil, nil, forbiddenRoles())
		_, err := svc.GetMilestone(context.Background(), testutil.MakeUser(), "product-001", "milestone-001")
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

func TestUpdateMilestone(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		milestone := testutil.MakeMilestone()
		var updated *model.Milestone
		svc := newMilestoneService(
			&testutil.MockMilestoneStore{
				GetByIDFunc: func(_ context.Context, _ string) (*model.Milestone, error) { return milestone, nil },
				UpdateFunc:  func(_ context.Context, m *model.Milestone) error { updated = m; return nil },
			},
			nil, nil, nil, nil, nil, noopAudit(), ownerRoles(),
		)

		newName := "v2.0"
		result, err := svc.UpdateMilestone(context.Background(), testutil.MakeUser(), "product-001", "milestone-001", UpdateMilestoneInput{Name: &newName})
		if err != nil {
			t.Fatalf("UpdateMilestone: %v", err)
		}
		if result.Name != "v2.0" {
			t.Fatalf("Name = %q, want %q", result.Name, "v2.0")
		}
		if updated == nil {
			t.Fatal("milestone was not updated")
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newMilestoneService(nil, nil, nil, nil, nil, nil, nil, forbiddenRoles())
		_, err := svc.UpdateMilestone(context.Background(), testutil.MakeUser(), "product-001", "milestone-001", UpdateMilestoneInput{})
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

func TestDeleteMilestone(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		deleted := false
		svc := newMilestoneService(
			&testutil.MockMilestoneStore{
				DeleteFunc: func(_ context.Context, _ string) error { deleted = true; return nil },
			},
			nil, nil, nil, nil, nil, noopAudit(), ownerRoles(),
		)

		err := svc.DeleteMilestone(context.Background(), testutil.MakeUser(), "product-001", "milestone-001")
		if err != nil {
			t.Fatalf("DeleteMilestone: %v", err)
		}
		if !deleted {
			t.Fatal("milestone was not deleted")
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newMilestoneService(nil, nil, nil, nil, nil, nil, nil, forbiddenRoles())
		err := svc.DeleteMilestone(context.Background(), testutil.MakeUser(), "product-001", "milestone-001")
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

func TestListMilestones(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		svc := newMilestoneService(
			&testutil.MockMilestoneStore{
				ListByProductIDFunc: func(_ context.Context, _ string) ([]*model.Milestone, error) {
					return []*model.Milestone{testutil.MakeMilestone()}, nil
				},
			},
			nil, nil, nil, nil, nil, nil, viewerRoles(),
		)

		result, err := svc.ListMilestones(context.Background(), testutil.MakeUser(), "product-001")
		if err != nil {
			t.Fatalf("ListMilestones: %v", err)
		}
		if len(result) != 1 {
			t.Fatalf("len(result) = %d, want 1", len(result))
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newMilestoneService(nil, nil, nil, nil, nil, nil, nil, forbiddenRoles())
		_, err := svc.ListMilestones(context.Background(), testutil.MakeUser(), "product-001")
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

// --- Sprints ---

func TestCreateSprint(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		var created *model.Sprint
		svc := newMilestoneService(nil,
			&testutil.MockSprintStore{
				CreateFunc: func(_ context.Context, s *model.Sprint) error { created = s; return nil },
			},
			nil, nil, nil, nil, noopAudit(), ownerRoles(),
		)

		result, err := svc.CreateSprint(context.Background(), testutil.MakeUser(), sprintInput("Sprint 1"))
		if err != nil {
			t.Fatalf("CreateSprint: %v", err)
		}
		if result.Name != "Sprint 1" {
			t.Fatalf("Name = %q, want %q", result.Name, "Sprint 1")
		}
		if result.Status != model.SprintStatusPlanned {
			t.Fatalf("Status = %q, want %q", result.Status, model.SprintStatusPlanned)
		}
		if created == nil {
			t.Fatal("sprint was not created")
		}
	})

	t.Run("missing name", func(t *testing.T) {
		svc := newMilestoneService(nil, nil, nil, nil, nil, nil, nil, ownerRoles())
		_, err := svc.CreateSprint(context.Background(), testutil.MakeUser(), CreateSprintInput{ProductID: "product-001"})
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("missing dates", func(t *testing.T) {
		svc := newMilestoneService(nil, nil, nil, nil, nil, nil, nil, ownerRoles())
		_, err := svc.CreateSprint(context.Background(), testutil.MakeUser(), CreateSprintInput{
			ProductID: "product-001", Name: "Sprint 1",
		})
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("end before start", func(t *testing.T) {
		svc := newMilestoneService(nil, nil, nil, nil, nil, nil, nil, ownerRoles())
		input := sprintInput("Sprint 1")
		input.StartDate, input.EndDate = input.EndDate, input.StartDate
		_, err := svc.CreateSprint(context.Background(), testutil.MakeUser(), input)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newMilestoneService(nil, nil, nil, nil, nil, nil, nil, forbiddenRoles())
		_, err := svc.CreateSprint(context.Background(), testutil.MakeUser(), sprintInput("X"))
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

// sprintInput returns a valid CreateSprintInput; dates are required, so every
// success case needs them.
func sprintInput(name string) CreateSprintInput {
	sp := testutil.MakeSprint()
	return CreateSprintInput{
		ProductID: "product-001",
		Name:      name,
		StartDate: sp.StartDate,
		EndDate:   sp.EndDate,
	}
}

func TestSetSprintStatus(t *testing.T) {
	t.Run("starts a planned sprint", func(t *testing.T) {
		sprint := testutil.MakeSprint()
		var updated *model.Sprint
		svc := newMilestoneService(nil,
			&testutil.MockSprintStore{
				GetByIDFunc:   func(_ context.Context, _ string) (*model.Sprint, error) { return sprint, nil },
				UpdateFunc:    func(_ context.Context, s *model.Sprint) error { updated = s; return nil },
				GetActiveFunc: func(_ context.Context, _ string) (*model.Sprint, error) { return nil, nil },
			},
			nil, nil, nil, nil, noopAudit(), ownerRoles(),
		)

		result, err := svc.SetSprintStatus(context.Background(), testutil.MakeUser(), "product-001", "sprint-001", model.SprintStatusActive)
		if err != nil {
			t.Fatalf("SetSprintStatus: %v", err)
		}
		if result.Status != model.SprintStatusActive {
			t.Fatalf("Status = %q, want %q", result.Status, model.SprintStatusActive)
		}
		if updated == nil {
			t.Fatal("sprint was not persisted")
		}
	})

	t.Run("rejects a second active sprint", func(t *testing.T) {
		sprint := testutil.MakeSprint()
		other := testutil.MakeSprint(func(s *model.Sprint) {
			s.ID = "sprint-002"
			s.Name = "Sprint 2"
			s.Status = model.SprintStatusActive
		})
		svc := newMilestoneService(nil,
			&testutil.MockSprintStore{
				GetByIDFunc:   func(_ context.Context, _ string) (*model.Sprint, error) { return sprint, nil },
				UpdateFunc:    func(_ context.Context, _ *model.Sprint) error { t.Fatal("must not persist"); return nil },
				GetActiveFunc: func(_ context.Context, _ string) (*model.Sprint, error) { return other, nil },
			},
			nil, nil, nil, nil, noopAudit(), ownerRoles(),
		)

		_, err := svc.SetSprintStatus(context.Background(), testutil.MakeUser(), "product-001", "sprint-001", model.SprintStatusActive)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("re-activating the same sprint is not a conflict", func(t *testing.T) {
		sprint := testutil.MakeSprint(func(s *model.Sprint) { s.Status = model.SprintStatusActive })
		svc := newMilestoneService(nil,
			&testutil.MockSprintStore{
				GetByIDFunc:   func(_ context.Context, _ string) (*model.Sprint, error) { return sprint, nil },
				UpdateFunc:    func(_ context.Context, _ *model.Sprint) error { return nil },
				GetActiveFunc: func(_ context.Context, _ string) (*model.Sprint, error) { return sprint, nil },
			},
			nil, nil, nil, nil, noopAudit(), ownerRoles(),
		)

		if _, err := svc.SetSprintStatus(context.Background(), testutil.MakeUser(), "product-001", "sprint-001", model.SprintStatusActive); err != nil {
			t.Fatalf("SetSprintStatus: %v", err)
		}
	})

	t.Run("closing needs no active check", func(t *testing.T) {
		sprint := testutil.MakeSprint(func(s *model.Sprint) { s.Status = model.SprintStatusActive })
		svc := newMilestoneService(nil,
			&testutil.MockSprintStore{
				GetByIDFunc: func(_ context.Context, _ string) (*model.Sprint, error) { return sprint, nil },
				UpdateFunc:  func(_ context.Context, _ *model.Sprint) error { return nil },
				GetActiveFunc: func(_ context.Context, _ string) (*model.Sprint, error) {
					t.Fatal("must not check for an active sprint when closing")
					return nil, nil
				},
			},
			nil, nil, nil, nil, noopAudit(), ownerRoles(),
		)

		result, err := svc.SetSprintStatus(context.Background(), testutil.MakeUser(), "product-001", "sprint-001", model.SprintStatusCompleted)
		if err != nil {
			t.Fatalf("SetSprintStatus: %v", err)
		}
		if result.Status != model.SprintStatusCompleted {
			t.Fatalf("Status = %q, want %q", result.Status, model.SprintStatusCompleted)
		}
	})

	t.Run("rejects unknown status", func(t *testing.T) {
		sprint := testutil.MakeSprint()
		svc := newMilestoneService(nil,
			&testutil.MockSprintStore{
				GetByIDFunc: func(_ context.Context, _ string) (*model.Sprint, error) { return sprint, nil },
			},
			nil, nil, nil, nil, noopAudit(), ownerRoles(),
		)

		_, err := svc.SetSprintStatus(context.Background(), testutil.MakeUser(), "product-001", "sprint-001", "paused")
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("rejects a sprint from another product", func(t *testing.T) {
		sprint := testutil.MakeSprint(func(s *model.Sprint) { s.ProductID = "product-002" })
		svc := newMilestoneService(nil,
			&testutil.MockSprintStore{
				GetByIDFunc: func(_ context.Context, _ string) (*model.Sprint, error) { return sprint, nil },
			},
			nil, nil, nil, nil, noopAudit(), ownerRoles(),
		)

		_, err := svc.SetSprintStatus(context.Background(), testutil.MakeUser(), "product-001", "sprint-001", model.SprintStatusActive)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newMilestoneService(nil, nil, nil, nil, nil, nil, nil, forbiddenRoles())
		_, err := svc.SetSprintStatus(context.Background(), testutil.MakeUser(), "product-001", "sprint-001", model.SprintStatusActive)
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

func TestGetSprint(t *testing.T) {
	sprint := testutil.MakeSprint()

	t.Run("success", func(t *testing.T) {
		svc := newMilestoneService(nil,
			&testutil.MockSprintStore{
				GetByIDFunc: func(_ context.Context, _ string) (*model.Sprint, error) { return sprint, nil },
			},
			nil, nil, nil, nil, nil, viewerRoles(),
		)

		result, err := svc.GetSprint(context.Background(), testutil.MakeUser(), "product-001", "sprint-001")
		if err != nil {
			t.Fatalf("GetSprint: %v", err)
		}
		if result.ID != sprint.ID {
			t.Fatalf("ID = %q, want %q", result.ID, sprint.ID)
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newMilestoneService(nil, nil, nil, nil, nil, nil, nil, forbiddenRoles())
		_, err := svc.GetSprint(context.Background(), testutil.MakeUser(), "product-001", "sprint-001")
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

func TestUpdateSprint(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		sprint := testutil.MakeSprint()
		var updated *model.Sprint
		svc := newMilestoneService(nil,
			&testutil.MockSprintStore{
				GetByIDFunc: func(_ context.Context, _ string) (*model.Sprint, error) { return sprint, nil },
				UpdateFunc:  func(_ context.Context, s *model.Sprint) error { updated = s; return nil },
			},
			nil, nil, nil, nil, noopAudit(), ownerRoles(),
		)

		newName := "Sprint 2"
		result, err := svc.UpdateSprint(context.Background(), testutil.MakeUser(), "product-001", "sprint-001", UpdateSprintInput{Name: &newName})
		if err != nil {
			t.Fatalf("UpdateSprint: %v", err)
		}
		if result.Name != "Sprint 2" {
			t.Fatalf("Name = %q, want %q", result.Name, "Sprint 2")
		}
		if updated == nil {
			t.Fatal("sprint was not updated")
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newMilestoneService(nil, nil, nil, nil, nil, nil, nil, forbiddenRoles())
		_, err := svc.UpdateSprint(context.Background(), testutil.MakeUser(), "product-001", "sprint-001", UpdateSprintInput{})
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

func TestDeleteSprint(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		deleted := false
		svc := newMilestoneService(nil,
			&testutil.MockSprintStore{
				DeleteFunc: func(_ context.Context, _ string) error { deleted = true; return nil },
			},
			nil, nil, nil, nil, noopAudit(), ownerRoles(),
		)

		err := svc.DeleteSprint(context.Background(), testutil.MakeUser(), "product-001", "sprint-001")
		if err != nil {
			t.Fatalf("DeleteSprint: %v", err)
		}
		if !deleted {
			t.Fatal("sprint was not deleted")
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newMilestoneService(nil, nil, nil, nil, nil, nil, nil, forbiddenRoles())
		err := svc.DeleteSprint(context.Background(), testutil.MakeUser(), "product-001", "sprint-001")
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

func TestListSprints(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		svc := newMilestoneService(nil,
			&testutil.MockSprintStore{
				ListByProductIDFunc: func(_ context.Context, _ string) ([]*model.Sprint, error) {
					return []*model.Sprint{testutil.MakeSprint()}, nil
				},
			},
			nil, nil, nil, nil, nil, viewerRoles(),
		)

		result, err := svc.ListSprints(context.Background(), testutil.MakeUser(), "product-001")
		if err != nil {
			t.Fatalf("ListSprints: %v", err)
		}
		if len(result) != 1 {
			t.Fatalf("len(result) = %d, want 1", len(result))
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newMilestoneService(nil, nil, nil, nil, nil, nil, nil, forbiddenRoles())
		_, err := svc.ListSprints(context.Background(), testutil.MakeUser(), "product-001")
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

// --- Statuses ---

func TestCreateStatus(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		var created *model.Status
		svc := newMilestoneService(nil, nil,
			&testutil.MockStatusStore{
				CreateFunc: func(_ context.Context, s *model.Status) error { created = s; return nil },
			},
			nil, nil, nil, noopAudit(), ownerRoles(),
		)

		result, err := svc.CreateStatus(context.Background(), testutil.MakeUser(), CreateStatusInput{
			ProductID: "product-001", EntityType: "requirement", Name: "Open",
		})
		if err != nil {
			t.Fatalf("CreateStatus: %v", err)
		}
		if result.Name != "Open" {
			t.Fatalf("Name = %q, want %q", result.Name, "Open")
		}
		if created == nil {
			t.Fatal("status was not created")
		}
	})

	t.Run("missing name", func(t *testing.T) {
		svc := newMilestoneService(nil, nil, nil, nil, nil, nil, nil, ownerRoles())
		_, err := svc.CreateStatus(context.Background(), testutil.MakeUser(), CreateStatusInput{ProductID: "product-001", EntityType: "requirement"})
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("missing entity type", func(t *testing.T) {
		svc := newMilestoneService(nil, nil, nil, nil, nil, nil, nil, ownerRoles())
		_, err := svc.CreateStatus(context.Background(), testutil.MakeUser(), CreateStatusInput{ProductID: "product-001", Name: "Open"})
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newMilestoneService(nil, nil, nil, nil, nil, nil, nil, forbiddenRoles())
		_, err := svc.CreateStatus(context.Background(), testutil.MakeUser(), CreateStatusInput{ProductID: "product-001", EntityType: "requirement", Name: "X"})
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

func TestGetStatus(t *testing.T) {
	status := testutil.MakeStatus()

	t.Run("success", func(t *testing.T) {
		svc := newMilestoneService(nil, nil,
			&testutil.MockStatusStore{
				GetByIDFunc: func(_ context.Context, _ string) (*model.Status, error) { return status, nil },
			},
			nil, nil, nil, nil, viewerRoles(),
		)

		result, err := svc.GetStatus(context.Background(), testutil.MakeUser(), "product-001", "status-001")
		if err != nil {
			t.Fatalf("GetStatus: %v", err)
		}
		if result.ID != status.ID {
			t.Fatalf("ID = %q, want %q", result.ID, status.ID)
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newMilestoneService(nil, nil, nil, nil, nil, nil, nil, forbiddenRoles())
		_, err := svc.GetStatus(context.Background(), testutil.MakeUser(), "product-001", "status-001")
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

func TestUpdateStatus(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		status := testutil.MakeStatus()
		var updated *model.Status
		svc := newMilestoneService(nil, nil,
			&testutil.MockStatusStore{
				GetByIDFunc: func(_ context.Context, _ string) (*model.Status, error) { return status, nil },
				UpdateFunc:  func(_ context.Context, s *model.Status) error { updated = s; return nil },
			},
			nil, nil, nil, noopAudit(), ownerRoles(),
		)

		newName := "Closed"
		result, err := svc.UpdateStatus(context.Background(), testutil.MakeUser(), "product-001", "status-001", UpdateStatusInput{Name: &newName})
		if err != nil {
			t.Fatalf("UpdateStatus: %v", err)
		}
		if result.Name != "Closed" {
			t.Fatalf("Name = %q, want %q", result.Name, "Closed")
		}
		if updated == nil {
			t.Fatal("status was not updated")
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newMilestoneService(nil, nil, nil, nil, nil, nil, nil, forbiddenRoles())
		_, err := svc.UpdateStatus(context.Background(), testutil.MakeUser(), "product-001", "status-001", UpdateStatusInput{})
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

func TestDeleteStatus(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		deleted := false
		svc := newMilestoneService(nil, nil,
			&testutil.MockStatusStore{
				GetByIDFunc: func(_ context.Context, _ string) (*model.Status, error) {
					return testutil.MakeStatus(), nil
				},
				DeleteFunc: func(_ context.Context, _ string) error { deleted = true; return nil },
			},
			nil, nil, nil, noopAudit(), ownerRoles(),
		)

		err := svc.DeleteStatus(context.Background(), testutil.MakeUser(), "product-001", "status-001")
		if err != nil {
			t.Fatalf("DeleteStatus: %v", err)
		}
		if !deleted {
			t.Fatal("status was not deleted")
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newMilestoneService(nil, nil, nil, nil, nil, nil, nil, forbiddenRoles())
		err := svc.DeleteStatus(context.Background(), testutil.MakeUser(), "product-001", "status-001")
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

func TestListStatuses(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		svc := newMilestoneService(nil, nil,
			&testutil.MockStatusStore{
				ListByProductIDFunc: func(_ context.Context, _, _ string) ([]*model.Status, error) {
					return []*model.Status{testutil.MakeStatus()}, nil
				},
			},
			nil, nil, nil, nil, viewerRoles(),
		)

		result, err := svc.ListStatuses(context.Background(), testutil.MakeUser(), "product-001", "requirement")
		if err != nil {
			t.Fatalf("ListStatuses: %v", err)
		}
		if len(result) != 1 {
			t.Fatalf("len(result) = %d, want 1", len(result))
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newMilestoneService(nil, nil, nil, nil, nil, nil, nil, forbiddenRoles())
		_, err := svc.ListStatuses(context.Background(), testutil.MakeUser(), "product-001", "requirement")
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

// --- Pyramid Levels ---

func TestCreatePyramidLevel(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		var created *model.PyramidLevel
		svc := newMilestoneService(nil, nil, nil, nil,
			&testutil.MockPyramidLevelStore{
				CreateFunc: func(_ context.Context, pl *model.PyramidLevel) error { created = pl; return nil },
			},
			nil, noopAudit(), ownerRoles(),
		)

		result, err := svc.CreatePyramidLevel(context.Background(), testutil.MakeUser(), CreatePyramidLevelInput{
			ProductID: "product-001", Name: "Unit",
		})
		if err != nil {
			t.Fatalf("CreatePyramidLevel: %v", err)
		}
		if result.Name != "Unit" {
			t.Fatalf("Name = %q, want %q", result.Name, "Unit")
		}
		if created == nil {
			t.Fatal("pyramid level was not created")
		}
	})

	t.Run("missing name", func(t *testing.T) {
		svc := newMilestoneService(nil, nil, nil, nil, nil, nil, nil, ownerRoles())
		_, err := svc.CreatePyramidLevel(context.Background(), testutil.MakeUser(), CreatePyramidLevelInput{ProductID: "product-001"})
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newMilestoneService(nil, nil, nil, nil, nil, nil, nil, forbiddenRoles())
		_, err := svc.CreatePyramidLevel(context.Background(), testutil.MakeUser(), CreatePyramidLevelInput{ProductID: "product-001", Name: "X"})
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

func TestGetPyramidLevel(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		pl := &model.PyramidLevel{ID: "pl-001", ProductID: "product-001", Name: "Unit"}
		svc := newMilestoneService(nil, nil, nil, nil,
			&testutil.MockPyramidLevelStore{
				GetByIDFunc: func(_ context.Context, _ string) (*model.PyramidLevel, error) { return pl, nil },
			},
			nil, nil, viewerRoles(),
		)

		result, err := svc.GetPyramidLevel(context.Background(), testutil.MakeUser(), "product-001", "pl-001")
		if err != nil {
			t.Fatalf("GetPyramidLevel: %v", err)
		}
		if result.ID != pl.ID {
			t.Fatalf("ID = %q, want %q", result.ID, pl.ID)
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newMilestoneService(nil, nil, nil, nil, nil, nil, nil, forbiddenRoles())
		_, err := svc.GetPyramidLevel(context.Background(), testutil.MakeUser(), "product-001", "pl-001")
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

func TestUpdatePyramidLevel(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		pl := &model.PyramidLevel{ID: "pl-001", ProductID: "product-001", Name: "Unit"}
		var updated *model.PyramidLevel
		svc := newMilestoneService(nil, nil, nil, nil,
			&testutil.MockPyramidLevelStore{
				GetByIDFunc: func(_ context.Context, _ string) (*model.PyramidLevel, error) { return pl, nil },
				UpdateFunc:  func(_ context.Context, p *model.PyramidLevel) error { updated = p; return nil },
			},
			nil, noopAudit(), ownerRoles(),
		)

		newName := "Integration"
		result, err := svc.UpdatePyramidLevel(context.Background(), testutil.MakeUser(), "product-001", "pl-001", UpdatePyramidLevelInput{Name: &newName})
		if err != nil {
			t.Fatalf("UpdatePyramidLevel: %v", err)
		}
		if result.Name != "Integration" {
			t.Fatalf("Name = %q, want %q", result.Name, "Integration")
		}
		if updated == nil {
			t.Fatal("pyramid level was not updated")
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newMilestoneService(nil, nil, nil, nil, nil, nil, nil, forbiddenRoles())
		_, err := svc.UpdatePyramidLevel(context.Background(), testutil.MakeUser(), "product-001", "pl-001", UpdatePyramidLevelInput{})
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

func TestDeletePyramidLevel(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		deleted := false
		svc := newMilestoneService(nil, nil, nil, nil,
			&testutil.MockPyramidLevelStore{
				DeleteFunc: func(_ context.Context, _ string) error { deleted = true; return nil },
			},
			nil, noopAudit(), ownerRoles(),
		)

		err := svc.DeletePyramidLevel(context.Background(), testutil.MakeUser(), "product-001", "pl-001")
		if err != nil {
			t.Fatalf("DeletePyramidLevel: %v", err)
		}
		if !deleted {
			t.Fatal("pyramid level was not deleted")
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newMilestoneService(nil, nil, nil, nil, nil, nil, nil, forbiddenRoles())
		err := svc.DeletePyramidLevel(context.Background(), testutil.MakeUser(), "product-001", "pl-001")
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

func TestListPyramidLevels(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		svc := newMilestoneService(nil, nil, nil, nil,
			&testutil.MockPyramidLevelStore{
				ListByProductIDFunc: func(_ context.Context, _ string) ([]*model.PyramidLevel, error) {
					return []*model.PyramidLevel{{ID: "pl-001"}}, nil
				},
			},
			nil, nil, viewerRoles(),
		)

		result, err := svc.ListPyramidLevels(context.Background(), testutil.MakeUser(), "product-001")
		if err != nil {
			t.Fatalf("ListPyramidLevels: %v", err)
		}
		if len(result) != 1 {
			t.Fatalf("len(result) = %d, want 1", len(result))
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newMilestoneService(nil, nil, nil, nil, nil, nil, nil, forbiddenRoles())
		_, err := svc.ListPyramidLevels(context.Background(), testutil.MakeUser(), "product-001")
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

// --- Runtime Environments ---

func TestCreateRuntimeEnvironment(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		var created *model.RuntimeEnvironment
		svc := newMilestoneService(nil, nil, nil, nil, nil,
			&testutil.MockRuntimeEnvironmentStore{
				CreateFunc: func(_ context.Context, env *model.RuntimeEnvironment) error { created = env; return nil },
			},
			noopAudit(), ownerRoles(),
		)

		result, err := svc.CreateRuntimeEnvironment(context.Background(), testutil.MakeUser(), "product-001", "staging")
		if err != nil {
			t.Fatalf("CreateRuntimeEnvironment: %v", err)
		}
		if result.Name != "staging" {
			t.Fatalf("Name = %q, want %q", result.Name, "staging")
		}
		if created == nil {
			t.Fatal("runtime environment was not created")
		}
	})

	t.Run("missing name", func(t *testing.T) {
		svc := newMilestoneService(nil, nil, nil, nil, nil, nil, nil, ownerRoles())
		_, err := svc.CreateRuntimeEnvironment(context.Background(), testutil.MakeUser(), "product-001", "")
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newMilestoneService(nil, nil, nil, nil, nil, nil, nil, forbiddenRoles())
		_, err := svc.CreateRuntimeEnvironment(context.Background(), testutil.MakeUser(), "product-001", "staging")
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

func TestDeleteRuntimeEnvironment(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		deleted := false
		svc := newMilestoneService(nil, nil, nil, nil, nil,
			&testutil.MockRuntimeEnvironmentStore{
				DeleteFunc: func(_ context.Context, _ string) error { deleted = true; return nil },
			},
			noopAudit(), ownerRoles(),
		)

		err := svc.DeleteRuntimeEnvironment(context.Background(), testutil.MakeUser(), "product-001", "env-001")
		if err != nil {
			t.Fatalf("DeleteRuntimeEnvironment: %v", err)
		}
		if !deleted {
			t.Fatal("runtime environment was not deleted")
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newMilestoneService(nil, nil, nil, nil, nil, nil, nil, forbiddenRoles())
		err := svc.DeleteRuntimeEnvironment(context.Background(), testutil.MakeUser(), "product-001", "env-001")
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

func TestListRuntimeEnvironments(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		svc := newMilestoneService(nil, nil, nil, nil, nil,
			&testutil.MockRuntimeEnvironmentStore{
				ListByProductIDFunc: func(_ context.Context, _ string) ([]*model.RuntimeEnvironment, error) {
					return []*model.RuntimeEnvironment{{ID: "env-001", Name: "staging"}}, nil
				},
			},
			nil, viewerRoles(),
		)

		result, err := svc.ListRuntimeEnvironments(context.Background(), testutil.MakeUser(), "product-001")
		if err != nil {
			t.Fatalf("ListRuntimeEnvironments: %v", err)
		}
		if len(result) != 1 {
			t.Fatalf("len(result) = %d, want 1", len(result))
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newMilestoneService(nil, nil, nil, nil, nil, nil, nil, forbiddenRoles())
		_, err := svc.ListRuntimeEnvironments(context.Background(), testutil.MakeUser(), "product-001")
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}

func TestListSprintsByMilestone(t *testing.T) {
	t.Run("skips sprints from another product", func(t *testing.T) {
		svc := newMilestoneService(nil,
			&testutil.MockSprintStore{
				ListByMilestoneIDFunc: func(_ context.Context, milestoneID string) ([]*model.Sprint, error) {
					if milestoneID != "milestone-001" {
						t.Fatalf("milestoneID = %q, want milestone-001", milestoneID)
					}
					return []*model.Sprint{
						{ID: "sprint-001", ProductID: "product-001", Name: "Sprint 1"},
						{ID: "sprint-002", ProductID: "other-product", Name: "Sprint 2"},
					}, nil
				},
			},
			nil, nil, nil, nil, nil, viewerRoles(),
		)

		result, err := svc.ListSprintsByMilestone(context.Background(), testutil.MakeUser(), "product-001", "milestone-001")
		if err != nil {
			t.Fatalf("ListSprintsByMilestone: %v", err)
		}
		if len(result) != 1 || result[0].ID != "sprint-001" {
			t.Fatalf("result = %v, want just sprint-001", result)
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		svc := newMilestoneService(nil, nil, nil, nil, nil, nil, nil, forbiddenRoles())
		_, err := svc.ListSprintsByMilestone(context.Background(), testutil.MakeUser(), "product-001", "milestone-001")
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("err = %v, want ErrForbidden", err)
		}
	})
}
