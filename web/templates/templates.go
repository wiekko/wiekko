// Package templates provides templ UI components for the Nexmap frontend.
package templates

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
	Body       string
	CreatedAt  string
	IsOwner    bool // whether the current user authored this comment
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
	ID       string
	Name     string
	IsClosed bool
}
