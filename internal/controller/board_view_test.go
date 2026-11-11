package controller

import (
	"testing"

	"nexmap.net/internal/model"
	"nexmap.net/web/templates"
)

func TestCanMoveCards(t *testing.T) {
	tests := []struct {
		name string
		user *model.User
		role string
		want bool
	}{
		{"admin", &model.User{IsAdmin: true}, "", true},
		{"admin with member role", &model.User{IsAdmin: true}, model.RoleViewer, true},
		{"owner", &model.User{}, model.RoleOwner, true},
		{"developer", &model.User{}, model.RoleDeveloper, true},
		{"tester", &model.User{}, model.RoleTester, false},
		{"viewer", &model.User{}, model.RoleViewer, false},
		{"no role", &model.User{}, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := canMoveCards(tt.user, tt.role); got != tt.want {
				t.Errorf("canMoveCards(%+v, %q) = %v, want %v", tt.user, tt.role, got, tt.want)
			}
		})
	}
}

func TestBuildBacklogCards_StrandsItemsWithoutStatus(t *testing.T) {
	prefixes := map[string]string{"cat-a": "FR"}
	reqs := []*model.Requirement{
		{ID: "r1", Kind: model.RequirementKindRequirement, CategoryID: "cat-a", SequenceNumber: 1, Title: "With status", StatusID: "s1", Priority: 3},
		{ID: "r2", Kind: model.RequirementKindRequirement, CategoryID: "cat-a", SequenceNumber: 2, Title: "Stranded", StatusID: "", Priority: 5},
	}

	byStatus, stranded, strandedTask := buildBacklogCards("p1", reqs, prefixes, true)

	if len(byStatus["s1"]) != 1 || byStatus["s1"][0].ID != "r1" {
		t.Errorf("expected r1 bucketed under s1, got %+v", byStatus)
	}
	if len(stranded) != 1 || stranded[0].ID != "r2" {
		t.Errorf("expected r2 in stranded, got %+v", stranded)
	}
	if len(strandedTask) != 0 {
		t.Errorf("expected no stranded tasks, got %+v", strandedTask)
	}
	if stranded[0].Identifier != "FR-2" {
		t.Errorf("expected stranded identifier FR-2, got %q", stranded[0].Identifier)
	}
	if byStatus["s1"][0].MoveURL != "/products/p1/board/requirements/r1/status" {
		t.Errorf("unexpected move URL: %q", byStatus["s1"][0].MoveURL)
	}
}

// Tasks share the requirement status columns but carry their own entity type,
// move URL and identifier prefix.
func TestBuildBacklogCards_TasksShareColumnsWithRequirements(t *testing.T) {
	prefixes := map[string]string{"cat-a": "FR", "cat-t": "MT"}
	items := []*model.Requirement{
		{ID: "r1", Kind: model.RequirementKindRequirement, CategoryID: "cat-a", SequenceNumber: 1, Title: "Requirement", StatusID: "s1"},
		{ID: "t1", Kind: model.RequirementKindTask, CategoryID: "cat-t", SequenceNumber: 2, Title: "Task", StatusID: "s1"},
		{ID: "t2", Kind: model.RequirementKindTask, CategoryID: "cat-t", SequenceNumber: 3, Title: "Stranded task", StatusID: ""},
	}

	byStatus, strandedReq, strandedTask := buildBacklogCards("p1", items, prefixes, true)

	if len(byStatus["s1"]) != 2 {
		t.Fatalf("expected requirement and task in the same column, got %+v", byStatus["s1"])
	}
	task := byStatus["s1"][1]
	if task.EntityType != model.RequirementKindTask {
		t.Errorf("expected task entity type, got %q", task.EntityType)
	}
	if task.Identifier != "MT-2" {
		t.Errorf("expected identifier MT-2, got %q", task.Identifier)
	}
	if task.MoveURL != "/products/p1/board/tasks/t1/status" {
		t.Errorf("unexpected task move URL: %q", task.MoveURL)
	}
	if len(strandedReq) != 0 {
		t.Errorf("expected no stranded requirements, got %+v", strandedReq)
	}
	if len(strandedTask) != 1 || strandedTask[0].ID != "t2" {
		t.Errorf("expected t2 in stranded tasks, got %+v", strandedTask)
	}
}

func TestBuildDefectCards_StrandsItemsWithoutStatus(t *testing.T) {
	defects := []*model.Defect{
		{ID: "d1", Title: "With status", StatusID: "s1", Priority: 2},
		{ID: "d2", Title: "Stranded", StatusID: "", Priority: 4},
	}

	byStatus, stranded := buildDefectCards("p1", defects, true)

	if len(byStatus["s1"]) != 1 || byStatus["s1"][0].ID != "d1" {
		t.Errorf("expected d1 bucketed under s1, got %+v", byStatus)
	}
	if len(stranded) != 1 || stranded[0].ID != "d2" {
		t.Errorf("expected d2 in stranded, got %+v", stranded)
	}
	if stranded[0].EntityType != "defect" {
		t.Errorf("expected entity type defect, got %q", stranded[0].EntityType)
	}
}

func TestMergedBoardColumns_PairsByPosition(t *testing.T) {
	reqStatuses := []*model.Status{
		{ID: "req-todo", Name: "To Do", Position: 1},
		{ID: "req-done", Name: "Done", Position: 2, IsClosed: true},
	}
	defStatuses := []*model.Status{
		{ID: "def-todo", Name: "To Do", Position: 1},
		{ID: "def-done", Name: "Done", Position: 2, IsClosed: true},
	}
	reqCards := map[string][]templates.BoardCard{
		"req-todo": {{ID: "r1", EntityType: "requirement"}},
	}
	defCards := map[string][]templates.BoardCard{
		"def-done": {{ID: "d1", EntityType: "defect"}},
	}

	cols := mergedBoardColumns(reqStatuses, defStatuses, reqCards, defCards)

	if len(cols) != 2 {
		t.Fatalf("expected 2 columns, got %d", len(cols))
	}

	todo := cols[0]
	if todo.Name != "To Do" || todo.ReqStatusID != "req-todo" || todo.DefectStatusID != "def-todo" {
		t.Errorf("unexpected To Do column: %+v", todo)
	}
	if len(todo.Cards) != 1 || todo.Cards[0].ID != "r1" {
		t.Errorf("To Do column should contain r1, got %+v", todo.Cards)
	}

	done := cols[1]
	if !done.IsClosed {
		t.Errorf("Done column should be marked closed")
	}
	if len(done.Cards) != 1 || done.Cards[0].ID != "d1" {
		t.Errorf("Done column should contain d1, got %+v", done.Cards)
	}
}

func TestMergedBoardColumns_HandlesMissingPair(t *testing.T) {
	// Simulate a drifted product where defect statuses don't match.
	reqStatuses := []*model.Status{
		{ID: "req-todo", Name: "To Do", Position: 1},
	}
	defStatuses := []*model.Status{
		{ID: "def-other", Name: "Other", Position: 99},
	}
	cols := mergedBoardColumns(reqStatuses, defStatuses, nil, nil)
	if len(cols) != 1 {
		t.Fatalf("expected 1 column, got %d", len(cols))
	}
	if cols[0].DefectStatusID != "" {
		t.Errorf("expected empty defect status id when no match, got %q", cols[0].DefectStatusID)
	}
}

func TestEntityBoardColumns_SplitMode(t *testing.T) {
	statuses := []*model.Status{
		{ID: "s1", Name: "Open", Position: 1},
		{ID: "s2", Name: "Closed", Position: 2, IsClosed: true},
	}
	cards := map[string][]templates.BoardCard{
		"s1": {{ID: "r1", EntityType: "requirement"}},
	}

	reqCols := entityBoardColumns(statuses, cards, true)
	if reqCols[0].ReqStatusID != "s1" || reqCols[0].DefectStatusID != "" {
		t.Errorf("requirement column should carry only ReqStatusID, got %+v", reqCols[0])
	}
	if len(reqCols[0].Cards) != 1 {
		t.Errorf("expected 1 card in Open column, got %d", len(reqCols[0].Cards))
	}
	if !reqCols[1].IsClosed {
		t.Errorf("closed column should be marked closed")
	}

	defCols := entityBoardColumns(statuses, nil, false)
	if defCols[0].DefectStatusID != "s1" || defCols[0].ReqStatusID != "" {
		t.Errorf("defect column should carry only DefectStatusID, got %+v", defCols[0])
	}
}

func TestBuildBoardSurface(t *testing.T) {
	reqStatuses := []*model.Status{{ID: "req-todo", Name: "To Do", Position: 1}}
	defStatuses := []*model.Status{{ID: "def-todo", Name: "To Do", Position: 1}}
	items := []*model.Requirement{
		{ID: "r1", Kind: model.RequirementKindRequirement, Title: "Req", StatusID: "req-todo"},
	}
	defects := []*model.Defect{{ID: "d1", Title: "Defect", StatusID: "def-todo"}}

	t.Run("sync mode merges both entity types into one column set", func(t *testing.T) {
		p := &model.Product{ID: "p1", StatusSync: true}
		surface := buildBoardSurface("p1", p, true, true, reqStatuses, defStatuses, items, defects, nil, "")

		if surface.Split {
			t.Fatal("Split should be false when StatusSync is on")
		}
		if len(surface.Columns) != 1 {
			t.Fatalf("expected 1 column, got %d", len(surface.Columns))
		}
		if len(surface.Columns[0].Cards) != 2 {
			t.Errorf("expected requirement and defect in one column, got %+v", surface.Columns[0].Cards)
		}
		if len(surface.ReqColumns) != 0 || len(surface.DefColumns) != 0 {
			t.Error("per-entity column sets should be empty in sync mode")
		}
	})

	t.Run("split mode keeps the entity types apart", func(t *testing.T) {
		p := &model.Product{ID: "p1", StatusSync: false}
		surface := buildBoardSurface("p1", p, true, true, reqStatuses, defStatuses, items, defects, nil, "sprint")

		if !surface.Split {
			t.Fatal("Split should be true when StatusSync is off")
		}
		if len(surface.ReqColumns) != 1 || len(surface.ReqColumns[0].Cards) != 1 {
			t.Errorf("unexpected requirement columns: %+v", surface.ReqColumns)
		}
		if len(surface.DefColumns) != 1 || len(surface.DefColumns[0].Cards) != 1 {
			t.Errorf("unexpected defect columns: %+v", surface.DefColumns)
		}
		if surface.GroupPrefix != "sprint" {
			t.Errorf("GroupPrefix = %q, want %q", surface.GroupPrefix, "sprint")
		}
	})

	t.Run("stranded cards surface regardless of mode", func(t *testing.T) {
		p := &model.Product{ID: "p1", StatusSync: true}
		stray := []*model.Requirement{
			{ID: "r9", Kind: model.RequirementKindTask, Title: "No status"},
		}
		surface := buildBoardSurface("p1", p, true, true, reqStatuses, defStatuses, stray, nil, nil, "")

		if len(surface.StrandedTask) != 1 || surface.StrandedTask[0].ID != "r9" {
			t.Errorf("expected r9 in stranded tasks, got %+v", surface.StrandedTask)
		}
	})
}

// FR-407: developers see inactive cards locked; owners can move every card.
// Kanban boards mark the active columns, scrum boards do not.
func TestBoardActivation(t *testing.T) {
	reqStatuses := []*model.Status{
		{ID: "new", Name: "New", Position: 1, IsDefault: true},
		{ID: "doing", Name: "Doing", Position: 2, IsActive: true},
	}
	items := []*model.Requirement{
		{ID: "idle", Kind: model.RequirementKindRequirement, StatusID: "new"},
		{ID: "busy", Kind: model.RequirementKindRequirement, StatusID: "doing", IsActive: true},
	}
	defects := []*model.Defect{{ID: "bug", StatusID: "new"}}
	kanban := &model.Product{ID: "p1", StatusSync: true, WorkflowMode: model.WorkflowModeKanban}

	locked := func(s templates.BoardSurfaceProps) map[string]bool {
		out := map[string]bool{}
		for _, col := range s.Columns {
			for _, c := range col.Cards {
				out[c.ID] = c.Locked
			}
		}
		return out
	}

	dev := buildBoardSurface("p1", kanban, true, false, reqStatuses, reqStatuses, items, defects, nil, "")
	if got := locked(dev); !got["idle"] || got["busy"] || !got["bug"] {
		t.Errorf("developer locks = %v, want idle and bug locked, busy movable", got)
	}
	if !dev.Columns[1].ShowActive || !dev.Columns[1].IsActive || dev.Columns[0].IsActive {
		t.Errorf("kanban columns = %+v, want Doing marked active", dev.Columns)
	}

	owner := buildBoardSurface("p1", kanban, true, true, reqStatuses, reqStatuses, items, defects, nil, "")
	for id, l := range locked(owner) {
		if l {
			t.Errorf("card %s locked for an owner", id)
		}
	}

	scrum := &model.Product{ID: "p1", StatusSync: true, WorkflowMode: model.WorkflowModeScrum}
	if s := buildBoardSurface("p1", scrum, true, false, reqStatuses, reqStatuses, items, nil, nil, "sprint"); s.Columns[1].ShowActive {
		t.Error("scrum board marks active columns")
	}
}
