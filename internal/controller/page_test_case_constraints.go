package controller

import (
	"net/http"
	"strings"

	"nexmap.net/internal/middleware"
	"nexmap.net/internal/model"
	"nexmap.net/internal/service"
	"nexmap.net/internal/store"
	"nexmap.net/web/templates"
	productpage "nexmap.net/web/templates/pages/product"
)

// Test case to constraint links (FR-150): the constraint's Test cases tab and
// the test case's Constraints tab. Linking needs the developer role, like
// test case to requirement links.

// ConstraintTestCasesTab renders GET /products/{id}/constraints/{constraintId}/tabs/test-cases.
func (c *PageProductController) ConstraintTestCasesTab(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	props, err := c.loadConstraintTestCasesTabProps(r, user, r.PathValue("id"), r.PathValue("constraintId"))
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.ConstraintTestCasesTabContent(props).Render(r.Context(), w)
}

// ConstraintTestCaseLinkModal renders GET /products/{id}/constraints/{constraintId}/test-cases/link-modal.
func (c *PageProductController) ConstraintTestCaseLinkModal(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	if !c.canLinkTestCases(r, user, productID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.TestCaseLinkModal(constraintTestCasesPath(productID, r.PathValue("constraintId"))+"/link-search").Render(r.Context(), w)
}

// ConstraintTestCaseLinkSearch renders GET /products/{id}/constraints/{constraintId}/test-cases/link-search:
// the test cases not linked to the constraint yet whose title matches q.
func (c *PageProductController) ConstraintTestCaseLinkSearch(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	constraintID := r.PathValue("constraintId")
	if !c.canLinkTestCases(r, user, productID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	query := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("q")))
	all, _ := c.testing.ListTestCases(r.Context(), user, productID)
	linkedIDs := map[string]bool{}
	if tcs, err := c.testing.ListConstraintTestCases(r.Context(), user, productID, constraintID); err == nil {
		for _, ev := range tcs.Linked {
			linkedIDs[ev.TestCase.ID] = true
		}
	}
	candidates := make([]*model.TestCase, 0, len(all))
	for _, tc := range all {
		if linkedIDs[tc.ID] || query != "" && !strings.Contains(strings.ToLower(tc.Title), query) {
			continue
		}
		candidates = append(candidates, tc)
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.TestCaseLinkResults(constraintTestCasesPath(productID, constraintID)+"/link", candidates).Render(r.Context(), w)
}

// ConstraintTestCaseLink handles POST /products/{id}/constraints/{constraintId}/test-cases/link.
func (c *PageProductController) ConstraintTestCaseLink(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	constraintID := r.PathValue("constraintId")
	testCaseID := r.FormValue("test_case_id")
	if testCaseID == "" {
		http.Error(w, "test_case_id is required", http.StatusBadRequest)
		return
	}
	if err := c.testing.LinkConstraint(r.Context(), user, productID, testCaseID, constraintID); err != nil {
		http.Error(w, errorText(err), http.StatusUnprocessableEntity)
		return
	}
	c.writeConstraintTestCasesTabOOB(w, r, user, productID, constraintID)
}

// ConstraintTestCaseCreateModal renders GET /products/{id}/constraints/{constraintId}/test-cases/create-modal.
func (c *PageProductController) ConstraintTestCaseCreateModal(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	p, _, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if !c.canLinkTestCases(r, user, productID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	c.renderTestCaseFormModal(w, r, user, productpage.TestCaseFormProps{
		Product: p,
		LinkTo:  "constraint:" + r.PathValue("constraintId"),
	})
}

// ConstraintTestCaseCreateAndLink handles POST /products/{id}/constraints/{constraintId}/test-cases:
// creates a test case and links it to the constraint.
func (c *PageProductController) ConstraintTestCaseCreateAndLink(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	constraintID := r.PathValue("constraintId")
	c.createTestCaseAndLink(w, r, user, productID, "constraint:"+constraintID,
		func(testCaseID string) error {
			return c.testing.LinkConstraint(r.Context(), user, productID, testCaseID, constraintID)
		},
		func() { c.writeConstraintTestCasesTabOOB(w, r, user, productID, constraintID) })
}

// ConstraintTestCaseUnlink handles DELETE /products/{id}/constraints/{constraintId}/test-cases/{testCaseId}.
func (c *PageProductController) ConstraintTestCaseUnlink(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	constraintID := r.PathValue("constraintId")
	if err := c.testing.UnlinkConstraint(r.Context(), user, productID, r.PathValue("testCaseId"), constraintID); err != nil {
		http.Error(w, errorText(err), http.StatusUnprocessableEntity)
		return
	}
	c.ConstraintTestCasesTab(w, r)
}

func (c *PageProductController) loadConstraintTestCasesTabProps(r *http.Request, user *model.User, productID, constraintID string) (productpage.ConstraintTestCasesTabProps, error) {
	_, role, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		return productpage.ConstraintTestCasesTabProps{}, err
	}
	tcs, err := c.testing.ListConstraintTestCases(r.Context(), user, productID, constraintID)
	if err != nil {
		return productpage.ConstraintTestCasesTabProps{}, err
	}
	props := productpage.ConstraintTestCasesTabProps{
		ProductID:    productID,
		ConstraintID: constraintID,
		CanWrite:     c.canLinkTestCasesForRole(user, role),
	}
	for _, ev := range tcs.Linked {
		props.Linked = append(props.Linked, productpage.TestCaseEvidenceView{TestCase: ev.TestCase, Latest: ev.Latest})
	}
	for _, d := range tcs.Derived {
		props.Derived = append(props.Derived, productpage.TestCaseEvidenceView{TestCase: d.TestCase, Latest: d.Latest, Via: d.Via})
	}
	return props, nil
}

func (c *PageProductController) writeConstraintTestCasesTabOOB(w http.ResponseWriter, r *http.Request, user *model.User, productID, constraintID string) {
	props, err := c.loadConstraintTestCasesTabProps(r, user, productID, constraintID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	c.writeTabOOB(w, r, "test-cases", productpage.ConstraintTestCasesTabContent(props))
}

func constraintTestCasesPath(productID, constraintID string) string {
	return "/products/" + productID + "/constraints/" + constraintID + "/test-cases"
}

// TestCaseConstraintsTab renders GET /products/{id}/test-cases/{testCaseId}/tabs/constraints.
func (c *PageProductController) TestCaseConstraintsTab(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	props, err := c.loadTestCaseConstraintsTabProps(r, user, r.PathValue("id"), r.PathValue("testCaseId"))
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.TestCaseConstraintsTabContent(props).Render(r.Context(), w)
}

// TestCaseConstraintLinkModal renders GET /products/{id}/test-cases/{testCaseId}/constraints/link-modal.
func (c *PageProductController) TestCaseConstraintLinkModal(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	if !c.canLinkTestCases(r, user, productID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.TestCaseConstraintLinkModal(productID, r.PathValue("testCaseId")).Render(r.Context(), w)
}

// TestCaseConstraintLinkSearch renders GET /products/{id}/test-cases/{testCaseId}/constraints/link-search:
// the constraints not linked to the test case yet whose identifier or title
// matches q.
func (c *PageProductController) TestCaseConstraintLinkSearch(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	testCaseID := r.PathValue("testCaseId")
	if !c.canLinkTestCases(r, user, productID) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}

	query := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("q")))
	all, _ := c.constraint.ListConstraints(r.Context(), user, productID, store.ConstraintFilter{})
	linked, _ := c.testing.ListTestCaseConstraints(r.Context(), user, productID, testCaseID)
	linkedIDs := make(map[string]bool, len(linked))
	for _, con := range linked {
		linkedIDs[con.ID] = true
	}
	prefixes := c.buildCategoryPrefixMap(r, user, productID, "constraint")
	candidates := make([]*model.Constraint, 0, len(all))
	for _, con := range all {
		if linkedIDs[con.ID] {
			continue
		}
		identifier := strings.ToLower(model.EntityIdentifier(prefixes[con.CategoryID], con.SequenceNumber))
		if query != "" && !strings.Contains(strings.ToLower(con.Title), query) && !strings.Contains(identifier, query) {
			continue
		}
		candidates = append(candidates, con)
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.TestCaseConstraintLinkResults(productID, testCaseID, candidates, prefixes).Render(r.Context(), w)
}

// TestCaseConstraintLink handles POST /products/{id}/test-cases/{testCaseId}/constraints/link.
func (c *PageProductController) TestCaseConstraintLink(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	productID := r.PathValue("id")
	testCaseID := r.PathValue("testCaseId")
	constraintID := r.FormValue("constraint_id")
	if constraintID == "" {
		http.Error(w, "constraint_id is required", http.StatusBadRequest)
		return
	}
	if err := c.testing.LinkConstraint(r.Context(), user, productID, testCaseID, constraintID); err != nil {
		http.Error(w, errorText(err), http.StatusUnprocessableEntity)
		return
	}
	props, err := c.loadTestCaseConstraintsTabProps(r, user, productID, testCaseID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	c.writeTabOOB(w, r, "constraints", productpage.TestCaseConstraintsTabContent(props))
}

// TestCaseConstraintUnlink handles DELETE /products/{id}/test-cases/{testCaseId}/constraints/{constraintId}.
func (c *PageProductController) TestCaseConstraintUnlink(w http.ResponseWriter, r *http.Request) {
	user := middleware.UserFromContext(r.Context())
	if err := c.testing.UnlinkConstraint(r.Context(), user, r.PathValue("id"), r.PathValue("testCaseId"), r.PathValue("constraintId")); err != nil {
		http.Error(w, errorText(err), http.StatusUnprocessableEntity)
		return
	}
	c.TestCaseConstraintsTab(w, r)
}

func (c *PageProductController) loadTestCaseConstraintsTabProps(r *http.Request, user *model.User, productID, testCaseID string) (productpage.TestCaseConstraintsTabProps, error) {
	_, role, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		return productpage.TestCaseConstraintsTabProps{}, err
	}
	constraints, err := c.testing.ListTestCaseConstraints(r.Context(), user, productID, testCaseID)
	if err != nil {
		return productpage.TestCaseConstraintsTabProps{}, err
	}
	return productpage.TestCaseConstraintsTabProps{
		ProductID:        productID,
		TestCaseID:       testCaseID,
		CanWrite:         c.canLinkTestCasesForRole(user, role),
		Constraints:      constraints,
		CategoryPrefixes: c.buildCategoryPrefixMap(r, user, productID, "constraint"),
	}, nil
}

// renderTestCaseFormModal renders the test case form as a modal that creates
// a test case and links it to props.LinkTo. It fills in the choices and the
// session's details.
func (c *PageProductController) renderTestCaseFormModal(w http.ResponseWriter, r *http.Request, user *model.User, props productpage.TestCaseFormProps) {
	productID := props.Product.ID
	props.Meta = templates.PageMeta{CSRFToken: middleware.CSRFTokenFromContext(r.Context())}
	props.DisplayName = user.DisplayName
	props.IsAdmin = user.IsAdmin
	props.Components, _ = c.component.ListComponents(r.Context(), user, productID)
	props.Capabilities, _ = c.component.ListCapabilities(r.Context(), user, productID)
	props.PyramidLevels, _ = c.milestone.ListPyramidLevels(r.Context(), user, productID)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = productpage.TestCaseFormModal(props).Render(r.Context(), w)
}

// createTestCaseAndLink creates a test case from the posted form and links it
// with link; done answers the request. A failure renders the form again with
// what was entered and the error.
func (c *PageProductController) createTestCaseAndLink(w http.ResponseWriter, r *http.Request, user *model.User, productID, linkTo string, link func(testCaseID string) error, done func()) {
	p, _, err := c.loadProductWithRole(r, user, productID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	in := service.CreateTestCaseInput{
		ProductID:      productID,
		Title:          r.FormValue("title"),
		Description:    r.FormValue("description"),
		ExecutionMode:  r.FormValue("execution_mode"),
		TestIdentifier: r.FormValue("test_identifier"),
		ComponentID:    r.FormValue("component_id"),
		CapabilityID:   r.FormValue("capability_id"),
		PyramidLevelID: r.FormValue("pyramid_level_id"),
	}
	rerender := func(errMsg string) {
		c.renderTestCaseFormModal(w, r, user, productpage.TestCaseFormProps{
			Product:        p,
			LinkTo:         linkTo,
			Title:          in.Title,
			Description:    in.Description,
			ExecutionMode:  in.ExecutionMode,
			TestIdentifier: in.TestIdentifier,
			ComponentID:    in.ComponentID,
			CapabilityID:   in.CapabilityID,
			PyramidLevelID: in.PyramidLevelID,
			Error:          errMsg,
		})
	}
	tc, err := c.testing.CreateTestCase(r.Context(), user, in)
	if err != nil {
		rerender(errorText(err))
		return
	}
	if err := link(tc.ID); err != nil {
		rerender("created test case but failed to link: " + errorText(err))
		return
	}
	done()
}
