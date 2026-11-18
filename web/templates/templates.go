// Package templates provides templ UI components for the Nexmap frontend.
package templates

import "strings"

// SelectOption represents a <select> option.
type SelectOption struct {
	Value    string
	Label    string
	Selected bool
}

// BreadcrumbItem represents one step in a breadcrumb trail.
type BreadcrumbItem struct {
	Label string
	Href  string // empty = current page (no link)
}

// TabDef defines a tab in a TabGroup.
type TabDef struct {
	ID     string
	Label  string
	HxGet  string // URL to load tab content via htmx
	Active bool
	// Count, when set, is shown in the label: the number of items a tab
	// listing links holds (UC-88). The tab's content keeps it current
	// through a data-tab-count attribute.
	Count *int
}

// WithCounts sets the Count of each tab named in counts. Tabs not named keep
// no count.
func WithCounts(tabs []TabDef, counts map[string]int) []TabDef {
	for i := range tabs {
		if n, ok := counts[tabs[i].ID]; ok {
			tabs[i].Count = &n
		}
	}
	return tabs
}

// PageMeta holds metadata for the base layout.
type PageMeta struct {
	Title     string
	CSRFToken string
}

// CommentView is a display-ready comment with author name resolved.
type CommentView struct {
	ID         string
	AuthorName string
	Body       string // Markdown; rendered by the template
	CreatedAt  string
	IsOwner    bool   // whether the current user authored this comment
	CanDelete  bool   // author, product owner or admin
	DeleteURL  string // empty when CanDelete is false
}

// AttachmentView is a display-ready attachment with a humanized size.
type AttachmentView struct {
	ID          string
	Filename    string
	ContentType string
	Size        string
	CreatedAt   string // already formatted for display
	DownloadURL string
	DeleteURL   string // empty when the viewer may not delete it
}

// Meta renders the secondary line under an attachment's filename, upload date
// first. Empty parts are skipped so a missing content type doesn't leave a
// dangling separator.
func (a AttachmentView) Meta() string {
	parts := make([]string, 0, 3)
	if a.CreatedAt != "" {
		parts = append(parts, a.CreatedAt)
	}
	if a.ContentType != "" {
		parts = append(parts, a.ContentType)
	}
	if a.Size != "" {
		parts = append(parts, a.Size)
	}
	return strings.Join(parts, " · ")
}

// AssigneeView is a display-ready assignee.
type AssigneeView struct {
	UserID      string
	DisplayName string
}

// MemberView is a display-ready product member for pickers.
type MemberView struct {
	UserID      string
	DisplayName string
	Role        string
}

// AuditEntryView is a display-ready audit trail entry.
type AuditEntryView struct {
	ID        string
	UserName  string
	Action    string
	FieldName string
	OldValue  string
	NewValue  string
	CreatedAt string
}

// StatusView is a display-ready status for dropdowns and badges.
type StatusView struct {
	ID        string
	Name      string
	IsDefault bool
	IsClosed  bool
}

// BoardSurfaceProps renders a board body without any page chrome, so the same
// markup serves the product board, the defects board and the sprint board tab.
type BoardSurfaceProps struct {
	ProductID string
	CanMove   bool

	// Split indicates separate status sets per entity type (StatusSync=false).
	// When false, Columns covers both entity types in one board.
	// When true, ReqColumns and DefColumns hold per-entity column sets.
	Split      bool
	Columns    []BoardColumn // populated when !Split
	ReqColumns []BoardColumn // populated when Split
	DefColumns []BoardColumn // populated when Split

	// Stranded cards lack a status and can't sit in any column. Shown as a
	// separate list below the board so users can still open & fix them.
	StrandedReq  []BoardCard
	StrandedTask []BoardCard
	StrandedDef  []BoardCard

	// GroupPrefix namespaces the Sortable drag groups, so two boards on one
	// page (board tab and planning tab) never accept each other's cards.
	GroupPrefix string
}

// BoardColumn represents one board column.
// ReqStatusID and DefectStatusID hold the per-entity-type status ids for the
// column. In sync mode both are set; in split mode only the relevant one.
type BoardColumn struct {
	Name           string
	ReqStatusID    string
	DefectStatusID string
	IsClosed       bool
	// IsActive marks a kanban column whose items are activated for work;
	// shown only when ShowActive is set (kanban boards), since scrum
	// activates by sprint instead.
	IsActive   bool
	ShowActive bool
	Cards      []BoardCard
}

// BoardCard carries the minimum info needed to display and move a card.
type BoardCard struct {
	EntityType string // "requirement", "task" or "defect"
	ID         string
	Identifier string // e.g. "FR-12" for requirements; "" for defects (no prefixed ID)
	Title      string
	Priority   int
	MoveURL    string
	// Locked is set when the viewer may move cards in general but not this
	// one: it has not been activated, and only owners move those (FR-116).
	Locked bool
}
