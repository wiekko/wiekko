package service

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strings"

	"nexmap.net/internal/model"
	"nexmap.net/internal/specdoc"
	"nexmap.net/internal/store"
)

// SpecificationService exports a product's specification as Markdown
// (FR-319, FR-905). Importing is deliberately not offered (FR-906).
type SpecificationService struct {
	products     store.ProductStore
	components   store.ComponentStore
	capabilities store.CapabilityStore
	categories   store.CategoryStore
	requirements store.RequirementStore
	constraints  store.ConstraintStore
	authz        *Authorizer
}

// ExportOptions selects what an export contains beyond the defaults.
type ExportOptions struct {
	Tasks bool // include tasks (FR-905: only on request)
}

// Export renders the product's structure, constraints and requirements, and
// tasks if asked for, as a Markdown specification. Everything is ordered by
// stable keys (category position, identifier number, name), never by
// priority or status, so that an unchanged product exports byte for byte the
// same. Requires viewer role: an export shows nothing a viewer cannot read.
func (s *SpecificationService) Export(ctx context.Context, user *model.User, productID string, opts ExportOptions) (string, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleViewer); err != nil {
		return "", err
	}
	product, err := s.products.GetByID(ctx, productID)
	if err != nil {
		return "", fmt.Errorf("%w: product", ErrNotFound)
	}
	doc := specdoc.Document{Product: product.Name, Description: product.Description}

	caps, err := s.capabilities.ListByProductID(ctx, productID)
	if err != nil {
		return "", fmt.Errorf("list capabilities: %w", err)
	}
	sortByName(caps, func(c *model.Capability) (string, string) { return c.Name, c.ID })
	if err := s.structure(ctx, productID, caps, &doc); err != nil {
		return "", err
	}
	reqCaps, conCaps, err := s.capabilityLinks(ctx, caps)
	if err != nil {
		return "", err
	}

	constraints, err := s.constraints.ListByProductID(ctx, productID, store.ConstraintFilter{})
	if err != nil {
		return "", fmt.Errorf("list constraints: %w", err)
	}
	conCats, err := s.categoriesOf(ctx, productID, "constraint")
	if err != nil {
		return "", err
	}
	var conItems []itemSource
	for _, c := range constraints {
		conItems = append(conItems, itemSource{id: c.ID, categoryID: c.CategoryID, seq: c.SequenceNumber,
			title: c.Title, description: c.Description, capabilities: conCaps[c.ID]})
	}
	doc.Constraints = buildSections(conCats, conItems)

	reqs, err := s.requirements.ListByProductID(ctx, productID, store.RequirementFilter{})
	if err != nil {
		return "", fmt.Errorf("list requirements: %w", err)
	}
	reqCats, err := s.categoriesOf(ctx, productID, model.RequirementKindRequirement)
	if err != nil {
		return "", err
	}
	taskCats, err := s.categoriesOf(ctx, productID, model.RequirementKindTask)
	if err != nil {
		return "", err
	}
	// Parents are referred to by identifier, whatever their kind, so the
	// prefixes of both kinds are needed even when tasks are left out.
	prefixes := map[string]string{}
	for _, c := range append(slices.Clone(reqCats), taskCats...) {
		prefixes[c.ID] = c.Prefix
	}
	var reqItems, taskItems []itemSource
	for _, r := range reqs {
		it := itemSource{id: r.ID, parentID: r.ParentID, categoryID: r.CategoryID, seq: r.SequenceNumber,
			title: r.Title, description: r.Description, capabilities: reqCaps[r.ID]}
		if r.Kind == model.RequirementKindTask {
			taskItems = append(taskItems, it)
			continue
		}
		it.definitionOfDone = r.DefinitionOfDone
		reqItems = append(reqItems, it)
	}
	identifiers := map[string]string{}
	for _, r := range reqs {
		identifiers[r.ID] = model.EntityIdentifier(prefixes[r.CategoryID], r.SequenceNumber)
	}
	doc.Requirements = buildSections(reqCats, withParents(reqItems, identifiers))
	if opts.Tasks {
		doc.Tasks = buildSections(taskCats, withParents(taskItems, identifiers))
	}
	return specdoc.Render(doc), nil
}

// structure fills in the components and the capabilities.
func (s *SpecificationService) structure(ctx context.Context, productID string, caps []*model.Capability, doc *specdoc.Document) error {
	components, err := s.components.ListByProductID(ctx, productID)
	if err != nil {
		return fmt.Errorf("list components: %w", err)
	}
	sortByName(components, func(c *model.Component) (string, string) { return c.Name, c.ID })
	byComponent := map[string][]specdoc.Capability{}
	known := map[string]bool{}
	for _, c := range components {
		known[c.ID] = true
	}
	for _, c := range caps {
		sc := specdoc.Capability{Name: c.Name, Description: c.Description}
		if c.ComponentID != "" && known[c.ComponentID] {
			byComponent[c.ComponentID] = append(byComponent[c.ComponentID], sc)
		} else {
			doc.Capabilities = append(doc.Capabilities, sc)
		}
	}
	for _, c := range components {
		doc.Components = append(doc.Components, specdoc.Component{
			Name: c.Name, Description: c.Description, Capabilities: byComponent[c.ID]})
	}
	return nil
}

// capabilityLinks maps requirement and constraint IDs to the names of the
// capabilities they are linked to, sorted by name.
func (s *SpecificationService) capabilityLinks(ctx context.Context, caps []*model.Capability) (reqCaps, conCaps map[string][]string, err error) {
	reqCaps, conCaps = map[string][]string{}, map[string][]string{}
	for _, c := range caps { // caps is sorted by name, so the lists are too
		reqLinks, err := s.capabilities.ListRequirementsByCapability(ctx, c.ID)
		if err != nil {
			return nil, nil, fmt.Errorf("list capability links: %w", err)
		}
		for _, l := range reqLinks {
			reqCaps[l.RequirementID] = append(reqCaps[l.RequirementID], c.Name)
		}
		conLinks, err := s.constraints.ListConstraintsByCapability(ctx, c.ID)
		if err != nil {
			return nil, nil, fmt.Errorf("list capability links: %w", err)
		}
		for _, l := range conLinks {
			conCaps[l.ConstraintID] = append(conCaps[l.ConstraintID], c.Name)
		}
	}
	return reqCaps, conCaps, nil
}

// categoriesOf lists the product's categories of one entity type in their
// configured order.
func (s *SpecificationService) categoriesOf(ctx context.Context, productID, entityType string) ([]*model.Category, error) {
	cats, err := s.categories.ListByProductID(ctx, productID, entityType)
	if err != nil {
		return nil, fmt.Errorf("list %s categories: %w", entityType, err)
	}
	slices.SortStableFunc(cats, func(a, b *model.Category) int {
		return cmp.Or(cmp.Compare(a.Position, b.Position), cmp.Compare(a.Prefix, b.Prefix))
	})
	return cats, nil
}

// itemSource is a requirement, task or constraint on its way into a section.
type itemSource struct {
	id, parentID, categoryID string
	seq                      int
	title, description       string
	definitionOfDone         string
	capabilities             []string
	parentIdentifier         string // set by withParents
}

// withParents records each item's parent identifier.
func withParents(items []itemSource, identifiers map[string]string) []itemSource {
	for i := range items {
		if items[i].parentID != "" {
			items[i].parentIdentifier = identifiers[items[i].parentID]
		}
	}
	return items
}

// buildSections groups items by category, in category order, and nests an
// item below its parent when both are in the same section; otherwise the
// item stands on its own and names the parent it refines. Items are ordered
// by number. Categories without items are left out; items whose category is
// unknown are collected in a final "Uncategorized" section rather than
// dropped.
func buildSections(cats []*model.Category, items []itemSource) []specdoc.Section {
	slices.SortFunc(items, func(a, b itemSource) int { return cmp.Compare(a.seq, b.seq) })
	known := map[string]*model.Category{}
	for _, c := range cats {
		known[c.ID] = c
	}
	sectionOf := func(it itemSource) string {
		if _, ok := known[it.categoryID]; ok {
			return it.categoryID
		}
		return ""
	}
	inSection := map[string]itemSource{}
	for _, it := range items {
		inSection[it.id] = it
	}
	children := map[string][]itemSource{}
	roots := map[string][]itemSource{}
	for _, it := range items {
		if p, ok := inSection[it.parentID]; ok && it.parentID != "" && sectionOf(p) == sectionOf(it) {
			children[it.parentID] = append(children[it.parentID], it)
			continue
		}
		roots[sectionOf(it)] = append(roots[sectionOf(it)], it)
	}

	var build func(it itemSource, nested bool, seen map[string]bool) specdoc.Item
	build = func(it itemSource, nested bool, seen map[string]bool) specdoc.Item {
		prefix := ""
		if c, ok := known[it.categoryID]; ok {
			prefix = c.Prefix
		}
		out := specdoc.Item{
			Identifier: model.EntityIdentifier(cmp.Or(prefix, "?"), it.seq), Title: it.title,
			Description: it.description, DefinitionOfDone: it.definitionOfDone, Capabilities: it.capabilities,
		}
		if !nested {
			out.Refines = it.parentIdentifier
		}
		seen[it.id] = true // parents cannot form a cycle (FR-008c); this keeps a corrupt one finite
		for _, c := range children[it.id] {
			if !seen[c.id] {
				out.Children = append(out.Children, build(c, true, seen))
			}
		}
		return out
	}

	var sections []specdoc.Section
	add := func(name, key string) {
		var list []specdoc.Item
		for _, it := range roots[key] {
			list = append(list, build(it, false, map[string]bool{}))
		}
		if len(list) > 0 {
			sections = append(sections, specdoc.Section{Category: name, Items: list})
		}
	}
	for _, c := range cats {
		add(c.Name, c.ID)
	}
	add("Uncategorized", "")
	return sections
}

// sortByName orders by case-insensitive name, then by ID, so the order does
// not depend on the database's collation.
func sortByName[T any](list []T, key func(T) (name, id string)) {
	slices.SortStableFunc(list, func(a, b T) int {
		an, aid := key(a)
		bn, bid := key(b)
		return cmp.Or(cmp.Compare(strings.ToLower(an), strings.ToLower(bn)), cmp.Compare(an, bn), cmp.Compare(aid, bid))
	})
}
