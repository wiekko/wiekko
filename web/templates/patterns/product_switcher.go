package patterns

import "nexmap.net/internal/model"

// productSections are the AppLayout ActiveSection values that map directly onto
// a /products/{id}/<section> URL. Detail and form pages set the same section as
// their list page, so they degrade to the list URL for free — the entity id is
// meaningless in another product.
var productSections = map[string]bool{
	"requirements": true,
	"defects":      true,
	"tasks":        true,
	"test-cases":   true,
	"campaigns":    true,
	"components":   true,
	"capabilities": true,
	"constraints":  true,
	"documents":    true,
	"milestones":   true,
	"team":         true,
	"settings":     true,
}

// SwitchTargetURL returns where the given section should land in the target
// product, so that switching product preserves the user's place.
//
// Sections that do not exist under a product (dashboard, admin-*, user-*, empty) and
// unrecognised input fall back to the target's overview. The section string
// reaches us as a query parameter, so the URL is always built from the matched
// map key rather than from the raw input.
func SwitchTargetURL(productID, workflowMode, section string) string {
	overview := "/products/" + productID

	switch {
	case productSections[section]:
		// Settings for a non-owner is redirected to the overview by the
		// settings handler itself, so preserving it needs no role check here.
		return overview + "/" + section

	case section == "board" || section == "sprints":
		// The workflow surface is one page or the other, so resolve against
		// the target product's mode rather than the source's.
		if workflowMode == model.WorkflowModeScrum {
			return overview + "/sprints"
		}
		return overview + "/board"

	default:
		return overview
	}
}
