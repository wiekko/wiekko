package ui

import (
	"strings"

	"github.com/a-h/templ"
)

// ConfirmVariant selects the styling of a confirmation dialog (UR-154).
type ConfirmVariant string

const (
	// ConfirmNeutral is for actions that can be reverted, such as unlinking.
	ConfirmNeutral ConfirmVariant = "neutral"
	// ConfirmDanger is for actions that cannot be undone, such as deleting.
	ConfirmDanger ConfirmVariant = "danger"
)

// Confirm returns the attributes that make an htmx request ask for
// confirmation first. Spread them onto the triggering element:
//
//	<button hx-delete={ url } { ui.Confirm(ui.ConfirmNeutral, "Unlink requirement", "Unlink", msg)... }>
//
// confirm.js shows the application's confirmation modal for them instead of
// the browser's confirm(). The message names the item concerned and, for
// ConfirmDanger, says that the action cannot be undone. Use this rather than
// a bare hx-confirm attribute.
func Confirm(variant ConfirmVariant, title, label, message string) templ.Attributes {
	return templ.Attributes{
		"hx-confirm":           message,
		"data-confirm-variant": string(variant),
		"data-confirm-title":   title,
		"data-confirm-label":   label,
	}
}

// ItemName names an item in a confirmation message: its identifier, if it
// has one, followed by its quoted title.
func ItemName(identifier, title string) string {
	quoted := "“" + strings.TrimSpace(title) + "”"
	if identifier == "" {
		return quoted
	}
	if strings.TrimSpace(title) == "" {
		return identifier
	}
	return identifier + " " + quoted
}
