package controller

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"nexmap.net/internal/middleware"
	"nexmap.net/internal/model"
	"nexmap.net/internal/service"
	"nexmap.net/internal/store"
	"nexmap.net/internal/textdiff"
	"nexmap.net/web/templates"
	productpage "nexmap.net/web/templates/pages/product"
)

// --- Document pages (FR-044..FR-052, FR-334) ---

// UseDocuments gives the product pages the document service.
func (c *PageProductController) UseDocuments(docs *service.DocumentService) { c.document = docs }

// canEditDocuments reports whether the user may write documents (owner or
// admin, FR-334).
func (c *PageProductController) canEditDocuments(user *model.User, role string) bool {
	return user.IsAdmin || role == model.RoleOwner
}

// formBody turns a submitted textarea into a document body. Browsers submit
// every line break of a textarea as CRLF; the body keeps LF line breaks
// unless it had CRLF ones before and the text is otherwise unchanged, so
// opening and saving the form never rewrites a body by itself.
func formBody(submitted, existing string) string {
	body := strings.ReplaceAll(submitted, "\r\n", "\n")
	if strings.Contains(existing, "\r\n") && strings.ReplaceAll(existing, "\r\n", "\n") == body {
		return existing
	}
	return body
}

// Documents renders GET /products/{id}/documents[?state=].
func (c *PageProductController) Documents(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	p, role, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	state := r.URL.Query().Get("state")
	if !model.ValidDocumentState(state) {
		state = ""
	}
	docs, _ := c.document.ListDocuments(r.Context(), user, productID, store.DocumentFilter{State: state}, store.ListParams{})
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.DocumentsPage(productpage.DocumentsProps{
		Meta:        templates.PageMeta{Title: p.Name + " - Documents", CSRFToken: middleware.CSRFTokenFromContext(r.Context())},
		DisplayName: user.DisplayName,
		IsAdmin:     user.IsAdmin,
		Product:     p,
		Documents:   docs,
		State:       state,
		CanEdit:     c.canEditDocuments(user, role),
	}).Render(r.Context(), w)
}

func (c *PageProductController) documentDetailProps(r *http.Request, user *model.User, p *model.Product, role string, d *model.Document) productpage.DocumentDetailProps {
	props := productpage.DocumentDetailProps{
		Meta:          templates.PageMeta{Title: p.Name + " - " + d.Title, CSRFToken: middleware.CSRFTokenFromContext(r.Context())},
		DisplayName:   user.DisplayName,
		IsAdmin:       user.IsAdmin,
		Product:       p,
		Document:      d,
		MilestoneName: c.resolveMilestoneName(r, user, p.ID, d.MilestoneID),
		CanEdit:       c.canEditDocuments(user, role),
	}
	if d.SupersededByID != "" {
		if next, err := c.document.GetDocument(r.Context(), user, p.ID, d.SupersededByID); err == nil {
			props.SupersededBy = next
		}
	}
	if d.ComponentID != "" {
		if comp, err := c.component.GetComponent(r.Context(), user, p.ID, d.ComponentID); err == nil {
			props.ComponentName = comp.Name
		}
	}
	if d.CapabilityID != "" {
		if capa, err := c.component.GetCapability(r.Context(), user, p.ID, d.CapabilityID); err == nil {
			props.CapabilityName = capa.Name
		}
	}
	return props
}

// DocumentDetail renders GET /products/{id}/documents/{documentId}.
func (c *PageProductController) DocumentDetail(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	p, role, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	d, err := c.document.GetDocument(r.Context(), user, productID, r.PathValue("documentId"))
	if err != nil {
		http.Redirect(w, r, "/products/"+productID+"/documents", http.StatusSeeOther)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.DocumentDetailPage(c.documentDetailProps(r, user, p, role, d)).Render(r.Context(), w)
}

// DocumentOverviewTab renders GET /products/{id}/documents/{documentId}/tabs/overview.
func (c *PageProductController) DocumentOverviewTab(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	p, role, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	d, err := c.document.GetDocument(r.Context(), user, productID, r.PathValue("documentId"))
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.DocumentOverviewTabContent(c.documentDetailProps(r, user, p, role, d)).Render(r.Context(), w)
}

// DocumentRaw serves GET /products/{id}/documents/{documentId}/raw: the body
// as a plain-text download, never rendered by the browser.
func (c *PageProductController) DocumentRaw(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	d, err := c.document.GetDocument(r.Context(), user, r.PathValue("id"), r.PathValue("documentId"))
	if err != nil {
		writePageServiceError(w, err)
		return
	}
	writeDocumentBody(w, model.DocumentFileName(d.Identifier, d.Format), d.Body)
}

// documentFormProps fills the form's choices.
func (c *PageProductController) documentFormProps(r *http.Request, user *model.User, p *model.Product, title string) productpage.DocumentFormProps {
	docs, _ := c.document.ListDocuments(r.Context(), user, p.ID, store.DocumentFilter{}, store.ListParams{})
	components, _ := c.component.ListComponents(r.Context(), user, p.ID)
	capabilities, _ := c.component.ListCapabilities(r.Context(), user, p.ID)
	milestones, _ := c.milestone.ListMilestones(r.Context(), user, p.ID)
	return productpage.DocumentFormProps{
		Meta:         templates.PageMeta{Title: p.Name + " - " + title, CSRFToken: middleware.CSRFTokenFromContext(r.Context())},
		DisplayName:  user.DisplayName,
		IsAdmin:      user.IsAdmin,
		Product:      p,
		Documents:    docs,
		Components:   components,
		Capabilities: capabilities,
		Milestones:   milestones,
	}
}

// formValues copies the submitted fields into the form props.
func documentFormValues(props *productpage.DocumentFormProps, r *http.Request, body string) {
	props.Title = r.FormValue("title")
	props.Body = body
	props.Format = r.FormValue("format")
	props.State = r.FormValue("state")
	props.SupersededByID = r.FormValue("superseded_by_id")
	props.ComponentID = r.FormValue("component_id")
	props.CapabilityID = r.FormValue("capability_id")
	props.MilestoneID = r.FormValue("milestone_id")
}

// DocumentCreateForm renders GET /products/{id}/documents/new.
func (c *PageProductController) DocumentCreateForm(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	p, _, err := c.loadProductWithRole(r, user, r.PathValue("id"))
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.DocumentFormPage(c.documentFormProps(r, user, p, "New Document")).Render(r.Context(), w)
}

// DocumentCreate handles POST /products/{id}/documents.
func (c *PageProductController) DocumentCreate(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	p, _, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	body := formBody(r.FormValue("body"), "")
	d, createErr := c.document.CreateDocument(r.Context(), user, service.CreateDocumentInput{
		ProductID: productID, Title: r.FormValue("title"), Body: body, Format: r.FormValue("format"),
		State: r.FormValue("state"), SupersededByID: r.FormValue("superseded_by_id"),
		ComponentID: r.FormValue("component_id"), CapabilityID: r.FormValue("capability_id"),
		MilestoneID: r.FormValue("milestone_id"),
	})
	if createErr != nil {
		props := c.documentFormProps(r, user, p, "New Document")
		documentFormValues(&props, r, body)
		props.Error = errorText(createErr)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusUnprocessableEntity)
		_ = productpage.DocumentFormPage(props).Render(r.Context(), w)
		return
	}
	http.Redirect(w, r, "/products/"+productID+"/documents/"+d.ID, http.StatusSeeOther)
}

// DocumentEditForm renders GET /products/{id}/documents/{documentId}/edit.
func (c *PageProductController) DocumentEditForm(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	p, _, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	d, err := c.document.GetDocument(r.Context(), user, productID, r.PathValue("documentId"))
	if err != nil {
		http.Redirect(w, r, "/products/"+productID+"/documents", http.StatusSeeOther)
		return
	}
	props := c.documentFormProps(r, user, p, "Edit "+d.Title)
	props.Document, props.IsEdit, props.ReturnTo = d, true, editReturnTarget(r)
	props.Title, props.Body, props.Format, props.State = d.Title, d.Body, d.Format, d.State
	props.SupersededByID, props.ComponentID, props.CapabilityID, props.MilestoneID = d.SupersededByID, d.ComponentID, d.CapabilityID, d.MilestoneID
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.DocumentFormPage(props).Render(r.Context(), w)
}

// DocumentUpdate handles POST /products/{id}/documents/{documentId}.
func (c *PageProductController) DocumentUpdate(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	documentID := r.PathValue("documentId")
	p, _, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	d, err := c.document.GetDocument(r.Context(), user, productID, documentID)
	if err != nil {
		http.Redirect(w, r, "/products/"+productID+"/documents", http.StatusSeeOther)
		return
	}
	title, format, state := r.FormValue("title"), r.FormValue("format"), r.FormValue("state")
	successor, component, capability, milestone := r.FormValue("superseded_by_id"), r.FormValue("component_id"), r.FormValue("capability_id"), r.FormValue("milestone_id")
	body := formBody(r.FormValue("body"), d.Body)
	_, updateErr := c.document.UpdateDocument(r.Context(), user, productID, documentID, service.UpdateDocumentInput{
		Title: &title, Body: &body, Format: &format, State: &state, SupersededByID: &successor,
		ComponentID: &component, CapabilityID: &capability, MilestoneID: &milestone,
	})
	if updateErr != nil {
		props := c.documentFormProps(r, user, p, "Edit "+d.Title)
		props.Document, props.IsEdit, props.ReturnTo = d, true, r.FormValue("return")
		documentFormValues(&props, r, body)
		props.Error = errorText(updateErr)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusUnprocessableEntity)
		_ = productpage.DocumentFormPage(props).Render(r.Context(), w)
		return
	}
	http.Redirect(w, r, postEditRedirect(r,
		"/products/"+productID+"/documents/"+documentID,
		"/products/"+productID+"/documents",
	), http.StatusSeeOther)
}

// DocumentDelete handles DELETE /products/{id}/documents/{documentId}.
func (c *PageProductController) DocumentDelete(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	if err := c.document.DeleteDocument(r.Context(), user, productID, r.PathValue("documentId")); err != nil {
		http.Error(w, errorText(err), http.StatusUnprocessableEntity)
		return
	}
	w.Header().Set("HX-Redirect", "/products/"+productID+"/documents")
	w.WriteHeader(http.StatusOK)
}

// DocumentDeleteConfirm renders GET /products/{id}/documents/{documentId}/delete/confirm.
func (c *PageProductController) DocumentDeleteConfirm(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	documentID := r.PathValue("documentId")
	d, err := c.document.GetDocument(r.Context(), user, productID, documentID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.DocumentDeleteConfirmation(d.Identifier+" "+d.Title, productID, documentID, middleware.CSRFTokenFromContext(r.Context())).Render(r.Context(), w)
}

// --- Links ---

var documentLinkPaths = map[string]string{
	model.DocumentLinkRequirement: "requirements",
	model.DocumentLinkConstraint:  "constraints",
	model.DocumentLinkTestCase:    "test-cases",
}

func (c *PageProductController) loadDocumentLinksProps(r *http.Request, user *model.User, productID, documentID string) (productpage.DocumentLinksTabProps, error) {
	_, role, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		return productpage.DocumentLinksTabProps{}, err
	}
	props := productpage.DocumentLinksTabProps{ProductID: productID, DocumentID: documentID, CanWrite: c.canEditDocuments(user, role)}
	for _, g := range []struct {
		typ  string
		into *[]productpage.DocumentLinkedItem
	}{
		{model.DocumentLinkRequirement, &props.Requirements},
		{model.DocumentLinkConstraint, &props.Constraints},
		{model.DocumentLinkTestCase, &props.TestCases},
	} {
		items, err := c.document.LinkedItems(r.Context(), user, productID, documentID, g.typ)
		if err != nil {
			return props, err
		}
		for _, it := range items {
			*g.into = append(*g.into, productpage.DocumentLinkedItem{ID: it.ID, Identifier: it.Identifier, Title: it.Title,
				Href: fmt.Sprintf("/products/%s/%s/%s", productID, documentLinkPaths[g.typ], it.ID)})
		}
	}
	return props, nil
}

// DocumentLinksTab renders GET /products/{id}/documents/{documentId}/tabs/links.
func (c *PageProductController) DocumentLinksTab(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	props, err := c.loadDocumentLinksProps(r, user, r.PathValue("id"), r.PathValue("documentId"))
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.DocumentLinksTabContent(props).Render(r.Context(), w)
}

// DocumentLinkModal renders GET /products/{id}/documents/{documentId}/links/link-modal.
func (c *PageProductController) DocumentLinkModal(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	if _, role, err := c.loadProductWithRole(r, user, productID); err != nil || !c.canEditDocuments(user, role) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.DocumentLinkModal(productID, r.PathValue("documentId"), middleware.CSRFTokenFromContext(r.Context())).Render(r.Context(), w)
}

// DocumentLinkSearch renders GET /products/{id}/documents/{documentId}/links/link-search?type=&q=:
// the items of one type not linked yet that match the query.
func (c *PageProductController) DocumentLinkSearch(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	documentID := r.PathValue("documentId")
	if _, role, err := c.loadProductWithRole(r, user, productID); err != nil || !c.canEditDocuments(user, role) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	targetType := r.URL.Query().Get("type")
	if !model.ValidDocumentLinkTarget(targetType) {
		targetType = model.DocumentLinkRequirement
	}
	linked := map[string]bool{}
	if links, err := c.document.ListLinks(r.Context(), user, productID, documentID); err == nil {
		for _, l := range links {
			linked[l.TargetID] = true
		}
	}
	var all []productpage.DocumentLinkedItem
	switch targetType {
	case model.DocumentLinkRequirement:
		reqs, _ := c.backlog.ListRequirements(r.Context(), user, productID, store.RequirementFilter{Kind: model.RequirementKindRequirement})
		prefixes := c.buildCategoryPrefixMap(r, user, productID, model.RequirementKindRequirement)
		for _, q := range reqs {
			all = append(all, productpage.DocumentLinkedItem{ID: q.ID, Identifier: model.EntityIdentifier(prefixes[q.CategoryID], q.SequenceNumber), Title: q.Title})
		}
	case model.DocumentLinkConstraint:
		cons, _ := c.constraint.ListConstraints(r.Context(), user, productID, store.ConstraintFilter{})
		prefixes := c.buildCategoryPrefixMap(r, user, productID, "constraint")
		for _, q := range cons {
			all = append(all, productpage.DocumentLinkedItem{ID: q.ID, Identifier: model.EntityIdentifier(prefixes[q.CategoryID], q.SequenceNumber), Title: q.Title})
		}
	case model.DocumentLinkTestCase:
		tcs, _ := c.testing.ListTestCases(r.Context(), user, productID)
		for _, q := range tcs {
			all = append(all, productpage.DocumentLinkedItem{ID: q.ID, Title: q.Title})
		}
	}
	query := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	candidates := make([]productpage.DocumentLinkedItem, 0, len(all))
	for _, it := range all {
		if linked[it.ID] {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(it.Identifier+" "+it.Title), query) {
			continue
		}
		candidates = append(candidates, it)
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.DocumentLinkResults(productID, documentID, targetType, candidates).Render(r.Context(), w)
}

// DocumentLink handles POST /products/{id}/documents/{documentId}/links with
// type and target_id, closing the modal and refreshing the links tab.
func (c *PageProductController) DocumentLink(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	documentID := r.PathValue("documentId")
	if err := c.document.LinkDocument(r.Context(), user, productID, documentID, r.FormValue("type"), r.FormValue("target_id")); err != nil {
		http.Error(w, errorText(err), http.StatusUnprocessableEntity)
		return
	}
	props, err := c.loadDocumentLinksProps(r, user, productID, documentID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	c.writeTabOOB(w, r, "links", productpage.DocumentLinksTabContent(props))
}

// DocumentUnlink handles DELETE /products/{id}/documents/{documentId}/links/{type}/{targetId}.
func (c *PageProductController) DocumentUnlink(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	documentID := r.PathValue("documentId")
	if err := c.document.UnlinkDocument(r.Context(), user, productID, documentID, r.PathValue("type"), r.PathValue("targetId")); err != nil {
		http.Error(w, errorText(err), http.StatusUnprocessableEntity)
		return
	}
	props, err := c.loadDocumentLinksProps(r, user, productID, documentID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.DocumentLinksTabContent(props).Render(r.Context(), w)
}

// --- Revisions ---

// DocumentRevisionsTab renders GET /products/{id}/documents/{documentId}/tabs/revisions[?from=&to=]:
// the revisions, and the difference between two of them (by default the
// latest and the one before).
func (c *PageProductController) DocumentRevisionsTab(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	documentID := r.PathValue("documentId")
	_, role, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	d, err := c.document.GetDocument(r.Context(), user, productID, documentID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	revs, err := c.document.ListRevisions(r.Context(), user, productID, documentID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	names := map[string]string{}
	if users, err := c.user.List(r.Context(), store.UserFilter{}); err == nil {
		for _, u := range users {
			names[u.ID] = u.DisplayName
		}
	}
	props := productpage.DocumentRevisionsTabProps{
		ProductID: productID, DocumentID: documentID, CSRFToken: middleware.CSRFTokenFromContext(r.Context()),
		Current: d.Revision, CanRestore: c.canEditDocuments(user, role),
	}
	for _, rev := range revs {
		props.Revisions = append(props.Revisions, productpage.DocumentRevisionRow{Revision: rev.Revision, Title: rev.Title,
			Format: rev.Format, Author: names[rev.UserID], CreatedAt: rev.CreatedAt})
	}
	props.To, props.From = d.Revision, d.Revision-1
	if n, err := strconv.Atoi(r.URL.Query().Get("to")); err == nil && n >= 1 {
		props.To = n
	}
	if n, err := strconv.Atoi(r.URL.Query().Get("from")); err == nil && n >= 1 {
		props.From = n
	}
	if len(revs) > 1 && props.From >= 1 {
		if diff, err := c.document.Diff(r.Context(), user, productID, documentID, props.From, props.To); err == nil {
			props.TitleChanged, props.FormatChanged = diff.TitleChanged, diff.FormatChanged
			props.Diff = documentDiffLines(diff.Lines)
		}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.DocumentRevisionsTabContent(props).Render(r.Context(), w)
}

// documentDiffLines keeps the changed lines with three lines of context,
// marking the unchanged stretches left out between hunks.
func documentDiffLines(lines []textdiff.Line) []productpage.DocumentDiffLine {
	var out []productpage.DocumentDiffLine
	for i, h := range textdiff.Hunks(lines, 3) {
		if i > 0 {
			out = append(out, productpage.DocumentDiffLine{Op: "gap"})
		}
		for _, l := range h.Lines {
			out = append(out, productpage.DocumentDiffLine{Op: diffOpName(l.Op), Text: l.Text, Old: l.Old, New: l.New})
		}
	}
	return out
}

// DocumentRestore handles POST /products/{id}/documents/{documentId}/restore.
func (c *PageProductController) DocumentRestore(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	documentID := r.PathValue("documentId")
	rev, err := strconv.Atoi(r.FormValue("revision"))
	if err != nil || rev < 1 {
		http.Error(w, "name the revision to restore", http.StatusBadRequest)
		return
	}
	if _, err := c.document.RestoreRevision(r.Context(), user, productID, documentID, rev); err != nil {
		http.Error(w, errorText(err), http.StatusUnprocessableEntity)
		return
	}
	w.Header().Set("HX-Redirect", "/products/"+productID+"/documents/"+documentID)
	w.WriteHeader(http.StatusOK)
}

// DocumentHistoryTab renders GET /products/{id}/documents/{documentId}/tabs/history.
func (c *PageProductController) DocumentHistoryTab(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	entries := c.buildAuditViews(r, user, r.PathValue("id"), "document", r.PathValue("documentId"))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.DocumentHistoryTabContent(entries).Render(r.Context(), w)
}

// linkedDocuments returns the documents linked to a requirement, constraint
// or test case, for its overview (FR-909); nil on any failure.
func (c *PageProductController) linkedDocuments(r *http.Request, user *model.User, productID, targetType, targetID string) []*model.Document {
	if c.document == nil {
		return nil
	}
	docs, err := c.document.DocumentsFor(r.Context(), user, productID, targetType, targetID)
	if err != nil {
		return nil
	}
	return docs
}

// Comments and attachments.

func (c *PageProductController) DocumentCommentsTab(w http.ResponseWriter, r *http.Request) {
	c.commentsTab(w, r, model.OwnerTypeDocument, r.PathValue("documentId"))
}
func (c *PageProductController) DocumentCommentCreate(w http.ResponseWriter, r *http.Request) {
	c.commentCreate(w, r, model.OwnerTypeDocument, r.PathValue("documentId"))
}
func (c *PageProductController) DocumentCommentDelete(w http.ResponseWriter, r *http.Request) {
	c.commentDelete(w, r, model.OwnerTypeDocument, r.PathValue("documentId"))
}
func (c *PageProductController) DocumentAttachmentsTab(w http.ResponseWriter, r *http.Request) {
	c.attachmentsTab(w, r, model.OwnerTypeDocument, r.PathValue("documentId"))
}
func (c *PageProductController) DocumentAttachmentCreate(w http.ResponseWriter, r *http.Request) {
	c.attachmentCreate(w, r, model.OwnerTypeDocument, r.PathValue("documentId"))
}
func (c *PageProductController) DocumentAttachmentDelete(w http.ResponseWriter, r *http.Request) {
	c.attachmentDelete(w, r, model.OwnerTypeDocument, r.PathValue("documentId"))
}
