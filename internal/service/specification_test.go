package service

import (
	"testing"

	"nexmap.net/internal/model"
)

func TestBuildSections(t *testing.T) {
	cats := []*model.Category{
		{ID: "fr", Name: "Functionality", Prefix: "FR"},
		{ID: "sr", Name: "Security", Prefix: "SR"},
		{ID: "ur", Name: "UX", Prefix: "UR"}, // no items: left out
	}
	items := withParents([]itemSource{
		{id: "c", parentID: "a", categoryID: "fr", seq: 3, title: "child"},
		{id: "a", categoryID: "fr", seq: 1, title: "root"},
		{id: "s", parentID: "a", categoryID: "sr", seq: 2, title: "other category"},
		{id: "x", categoryID: "gone", seq: 9, title: "lost category"},
		{id: "t", parentID: "missing", categoryID: "fr", seq: 4, title: "parent not exported"},
	}, map[string]string{"a": "FR-1", "missing": "MT-7"})

	got := buildSections(cats, items)
	if len(got) != 3 || got[0].Category != "Functionality" || got[1].Category != "Security" || got[2].Category != "Uncategorized" {
		t.Fatalf("sections = %+v", got)
	}
	fr := got[0].Items
	if len(fr) != 2 || fr[0].Identifier != "FR-1" || fr[1].Identifier != "FR-4" {
		t.Fatalf("Functionality roots = %+v; want FR-1 and FR-4 in number order", fr)
	}
	if len(fr[0].Children) != 1 || fr[0].Children[0].Identifier != "FR-3" || fr[0].Children[0].Refines != "" {
		t.Errorf("FR-3 should nest below FR-1 without a refines note: %+v", fr[0].Children)
	}
	if fr[1].Refines != "MT-7" {
		t.Errorf("an item whose parent is not exported names it: %+v", fr[1])
	}
	if sr := got[1].Items; len(sr) != 1 || sr[0].Refines != "FR-1" || len(sr[0].Children) != 0 {
		t.Errorf("an item in another category stands alone and names its parent: %+v", sr)
	}
	if lost := got[2].Items; len(lost) != 1 || lost[0].Identifier != "?-9" {
		t.Errorf("an item of an unknown category is kept: %+v", lost)
	}
}

func TestBuildSectionsSurvivesACycle(t *testing.T) {
	// Parent links cannot form a cycle (FR-12); a corrupt one must still
	// render rather than recurse forever. Items in a pure cycle have no root,
	// so they drop out instead of looping.
	cats := []*model.Category{{ID: "fr", Name: "Functionality", Prefix: "FR"}}
	items := []itemSource{
		{id: "a", parentID: "b", categoryID: "fr", seq: 1},
		{id: "b", parentID: "a", categoryID: "fr", seq: 2},
		{id: "r", categoryID: "fr", seq: 3},
	}
	got := buildSections(cats, items)
	if len(got) != 1 || len(got[0].Items) != 1 || got[0].Items[0].Identifier != "FR-3" {
		t.Fatalf("sections = %+v", got)
	}
}
