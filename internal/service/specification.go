// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package service

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"wiekko.net/internal/access"
	"wiekko.net/internal/model"
	"wiekko.net/internal/specdoc"
	"wiekko.net/internal/store"
)

// SpecificationService exports a product's specification as Markdown
// (FR-92, FR-137). Importing is deliberately not offered (FR-138).
type SpecificationService struct {
	products     store.ProductStore
	components   store.ComponentStore
	capabilities store.CapabilityStore
	categories   store.CategoryStore
	requirements store.RequirementStore
	constraints  store.ConstraintStore
	documents    store.DocumentStore
	testCases    store.TestCaseStore
	authz        *Authorizer
}

// ExportOptions selects what an export contains beyond the defaults.
type ExportOptions struct {
	Tasks bool // include tasks (FR-137: only on request)
	// Drafts includes draft documents besides the current ones (FR-139).
	Drafts bool
}

// ExportedDocuments lists the documents an export includes, in its order:
// the current ones, and the drafts when asked for. A token without the
// documents scope gets none; the export then leaves them out rather than
// failing (FR-139). Requires viewer role.
func (s *SpecificationService) ExportedDocuments(ctx context.Context, user *model.User, productID string, opts ExportOptions) ([]*model.Document, error) {
	if _, err := s.authz.RequireRole(ctx, user, productID, model.RoleViewer); err != nil {
		return nil, err
	}
	return s.exportedDocuments(ctx, productID, opts)
}

func (s *SpecificationService) exportedDocuments(ctx context.Context, productID string, opts ExportOptions) ([]*model.Document, error) {
	if s.documents == nil || !access.FromContext(ctx).Allows(access.Documents, access.Read) {
		return nil, nil
	}
	all, err := s.documents.ListPageByProductID(ctx, productID, store.DocumentFilter{}, store.ListParams{Sort: "identifier"})
	if err != nil {
		return nil, fmt.Errorf("list documents: %w", err)
	}
	docs := []*model.Document{}
	for _, d := range all {
		if d.State == model.DocumentStateCurrent || (opts.Drafts && d.State == model.DocumentStateDraft) {
			docs = append(docs, d)
		}
	}
	return docs, nil
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
	docs, err := s.exportedDocuments(ctx, productID, opts)
	if err != nil {
		return "", err
	}
	// docLinks[doc ID] are the document's links; docsOf[item ID] the
	// identifiers of the exported documents linked to the item.
	docLinks := map[string][]*model.DocumentLink{}
	docsOf := map[string][]string{}
	for _, d := range docs {
		links, err := s.documents.ListLinks(ctx, d.ID)
		if err != nil {
			return "", fmt.Errorf("list document links: %w", err)
		}
		docLinks[d.ID] = links
		for _, l := range links {
			docsOf[l.TargetID] = append(docsOf[l.TargetID], d.Identifier)
		}
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
	conPrefixes := map[string]string{}
	for _, c := range conCats {
		conPrefixes[c.ID] = c.Prefix
	}
	identifiers := map[string]string{}
	for _, c := range constraints {
		conItems = append(conItems, itemSource{id: c.ID, categoryID: c.CategoryID, seq: c.SequenceNumber,
			title: c.Title, description: c.Description, capabilities: conCaps[c.ID], documents: docsOf[c.ID]})
		identifiers[c.ID] = model.EntityIdentifier(conPrefixes[c.CategoryID], c.SequenceNumber)
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
			title: r.Title, description: r.Description, capabilities: reqCaps[r.ID], archived: r.Archived()}
		if r.Kind == model.RequirementKindTask {
			taskItems = append(taskItems, it)
			continue
		}
		it.documents = docsOf[r.ID]
		it.definitionOfDone = r.DefinitionOfDone
		reqItems = append(reqItems, it)
	}
	for _, r := range reqs {
		identifiers[r.ID] = model.EntityIdentifier(prefixes[r.CategoryID], r.SequenceNumber)
	}
	doc.Requirements = buildSections(reqCats, withParents(reqItems, identifiers))
	if opts.Tasks {
		doc.Tasks = buildSections(taskCats, withParents(taskItems, identifiers))
	}
	if docs != nil {
		if doc.Documents, err = s.documentEntries(ctx, docs, docLinks, identifiers); err != nil {
			return "", err
		}
	}
	return specdoc.Render(doc), nil
}

// documentEntries describes the exported documents with their links. Test
// cases are named only for a token that may read them.
func (s *SpecificationService) documentEntries(ctx context.Context, docs []*model.Document, docLinks map[string][]*model.DocumentLink, identifiers map[string]string) ([]specdoc.DocumentEntry, error) {
	tests := access.FromContext(ctx).Allows(access.Tests, access.Read)
	entries := []specdoc.DocumentEntry{}
	for _, d := range docs {
		e := specdoc.DocumentEntry{Identifier: d.Identifier, Title: d.Title, Format: d.Format, State: d.State,
			Revision: d.Revision, File: model.DocumentFileName(d.Identifier, d.Format)}
		for _, l := range docLinks[d.ID] {
			switch l.TargetType {
			case model.DocumentLinkRequirement:
				if id, ok := identifiers[l.TargetID]; ok {
					e.Requirements = append(e.Requirements, id)
				}
			case model.DocumentLinkConstraint:
				if id, ok := identifiers[l.TargetID]; ok {
					e.Constraints = append(e.Constraints, id)
				}
			case model.DocumentLinkTestCase:
				if !tests || s.testCases == nil {
					continue
				}
				tc, err := s.testCases.GetByID(ctx, l.TargetID)
				if err != nil {
					return nil, fmt.Errorf("read linked test case: %w", err)
				}
				e.TestCases = append(e.TestCases, tc.Title)
			}
		}
		sortIdentifiers(e.Requirements)
		sortIdentifiers(e.Constraints)
		slices.SortFunc(e.TestCases, func(a, b string) int {
			return cmp.Or(cmp.Compare(strings.ToLower(a), strings.ToLower(b)), cmp.Compare(a, b))
		})
		entries = append(entries, e)
	}
	return entries, nil
}

// sortIdentifiers orders identifiers like FR-2 < FR-10 < SR-1.
func sortIdentifiers(ids []string) {
	slices.SortFunc(ids, func(a, b string) int {
		ap, an, _ := strings.Cut(a, "-")
		bp, bn, _ := strings.Cut(b, "-")
		ai, _ := strconv.Atoi(an)
		bi, _ := strconv.Atoi(bn)
		return cmp.Or(cmp.Compare(ap, bp), cmp.Compare(ai, bi))
	})
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
	documents                []string // identifiers of linked documents
	parentIdentifier         string   // set by withParents
	archived                 bool     // FR-155
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
			Documents: it.documents, Archived: it.archived,
		}
		if !nested {
			out.Refines = it.parentIdentifier
		}
		seen[it.id] = true // parents cannot form a cycle (FR-12); this keeps a corrupt one finite
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
