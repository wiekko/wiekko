// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package router

import (
	"net/http"

	"nexmap.net/internal/access"
	"nexmap.net/internal/controller"
	"nexmap.net/internal/middleware"
	"nexmap.net/internal/model"
	"nexmap.net/internal/service"
)

// mountAPIv1 registers the REST API and returns the token scopes each
// authenticated route needs, keyed by route pattern.
func mountAPIv1(mux *http.ServeMux, svc *service.Services, cookieSecure bool) map[string][]access.Need {
	apiV1 := http.NewServeMux()
	api := &apiRoutes{mux: apiV1, needs: map[string][]access.Need{}}

	authCtrl := controller.NewAuth(svc.Auth, svc.User, cookieSecure)
	userCtrl := controller.NewUser(svc.User)
	patCtrl := controller.NewPAT(svc.PAT)
	totpCtrl := controller.NewTOTP(svc.TwoFactor)
	adminCtrl := controller.NewAdmin(svc.User, svc.Setting)
	invCtrl := controller.NewInvitation(svc.Invitation)
	productCtrl := controller.NewProduct(svc.Product, svc.User)
	componentCtrl := controller.NewComponent(svc.Component)
	planningCtrl := controller.NewPlanning(svc.Milestone)
	categoryCtrl := controller.NewCategory(svc.Category)
	backlogCtrl := controller.NewBacklog(svc.Backlog)
	constraintCtrl := controller.NewConstraint(svc.Constraint)
	testingCtrl := controller.NewTesting(svc.Testing, svc.Backlog.ResolveDefectRef)
	testRunCtrl := controller.NewTestRun(svc.TestRun)
	discussionCtrl := controller.NewDiscussion(svc.Discussion, svc.Backlog.ResolveDefectRef)
	auditCtrl := controller.NewAudit(svc.Audit)
	specCtrl := controller.NewSpecification(svc.Spec)
	documentCtrl := controller.NewDocument(svc.Document)
	backlogCtrl.UseDocuments(svc.Document)
	constraintCtrl.UseDocuments(svc.Document)
	testingCtrl.UseDocuments(svc.Document)
	testingCtrl.UseConstraints(svc.Constraint)

	// Requirement and constraint routes accept identifiers ("FR-12", "AC-42")
	// wherever they take an ID in the path.
	reqRef := controller.ResolveRefs(svc.Backlog.ResolveRequirementRef, "id")
	reqParamRef := controller.ResolveRefs(svc.Backlog.ResolveRequirementRef, "reqId")
	conRef := controller.ResolveRefs(svc.Constraint.ResolveConstraintRef, "id")
	conParamRef := controller.ResolveRefs(svc.Constraint.ResolveConstraintRef, "conId")
	// Defect routes accept defect identifiers ("DF-17", FR-49) likewise.
	defRef := controller.ResolveRefs(svc.Backlog.ResolveDefectRef, "id")
	// Document routes accept document identifiers ("DOC-3", FR-56), and
	// their link routes the identifiers of the linked items.
	docRef := controller.ResolveRefs(svc.Document.ResolveDocumentRef, "id")
	docTargetReqRef := controller.ResolveRefs(svc.Backlog.ResolveRequirementRef, "targetId")
	docTargetConRef := controller.ResolveRefs(svc.Constraint.ResolveConstraintRef, "targetId")

	// Health (public).
	apiV1.HandleFunc("GET /health", controller.Health)
	apiV1.HandleFunc("GET /version", controller.Version)

	// Auth (public).
	apiV1.HandleFunc("POST /auth/login", authCtrl.Login)
	apiV1.HandleFunc("POST /auth/login/totp", authCtrl.LoginTOTP)
	apiV1.HandleFunc("POST /auth/register", authCtrl.Register)
	apiV1.HandleFunc("POST /auth/accept-invite", authCtrl.AcceptInvite)

	// Auth (authenticated).
	api.authed("POST /auth/logout", noScope, csrf(http.HandlerFunc(authCtrl.Logout)))

	// Current user (authenticated).
	api.authed("GET /user/me", noScope, http.HandlerFunc(userCtrl.GetMe))
	api.authed("PATCH /user/me", need(access.W(access.Account)), csrf(http.HandlerFunc(userCtrl.UpdateMe)))
	api.authed("POST /user/me/password", need(access.W(access.Account)), csrf(http.HandlerFunc(userCtrl.ChangePassword)))
	api.authed("POST /user/me/delete", need(access.W(access.Account)), csrf(http.HandlerFunc(userCtrl.RequestDeletion)))

	// Personal access tokens (authenticated).
	// Two-factor authentication (authenticated).
	api.authed("GET /user/me/totp", need(access.R(access.Account)), http.HandlerFunc(totpCtrl.Status))
	api.authed("POST /user/me/totp", need(access.W(access.Account)), csrf(http.HandlerFunc(totpCtrl.Begin)))
	api.authed("POST /user/me/totp/confirm", need(access.W(access.Account)), csrf(http.HandlerFunc(totpCtrl.Confirm)))
	api.authed("POST /user/me/totp/recovery-codes", need(access.W(access.Account)), csrf(http.HandlerFunc(totpCtrl.Regenerate)))
	api.authed("DELETE /user/me/totp", need(access.W(access.Account)), csrf(http.HandlerFunc(totpCtrl.Disable)))

	api.authed("GET /user/me/tokens", need(access.R(access.Account)), http.HandlerFunc(patCtrl.List))
	api.authed("POST /user/me/tokens", need(access.W(access.Account)), csrf(http.HandlerFunc(patCtrl.Create)))
	api.authed("DELETE /user/me/tokens/{id}", need(access.W(access.Account)), csrf(http.HandlerFunc(patCtrl.Revoke)))

	// Admin: users (admin only).
	api.admin("GET /admin/users", need(access.R(access.Admin)), http.HandlerFunc(adminCtrl.ListUsers))
	api.admin("GET /admin/users/{id}", need(access.R(access.Admin)), http.HandlerFunc(adminCtrl.GetUser))
	api.admin("POST /admin/users/{id}/lock", need(access.W(access.Admin)), csrf(http.HandlerFunc(adminCtrl.LockUser)))
	api.admin("POST /admin/users/{id}/unlock", need(access.W(access.Admin)), csrf(http.HandlerFunc(adminCtrl.UnlockUser)))
	api.admin("POST /admin/users/{id}/password", need(access.W(access.Admin)), csrf(http.HandlerFunc(adminCtrl.SetPassword)))
	api.admin("POST /admin/users/{id}/approve", need(access.W(access.Admin)), csrf(http.HandlerFunc(adminCtrl.ApproveUser)))
	api.admin("DELETE /admin/users/{id}/totp", need(access.W(access.Admin)), csrf(http.HandlerFunc(totpCtrl.AdminReset)))
	api.admin("DELETE /admin/users/{id}", need(access.W(access.Admin)), csrf(http.HandlerFunc(adminCtrl.DeleteUser)))

	// Admin: settings (admin only).
	api.admin("GET /admin/settings", need(access.R(access.Admin)), http.HandlerFunc(adminCtrl.GetSettings))
	api.admin("PATCH /admin/settings", need(access.W(access.Admin)), csrf(http.HandlerFunc(adminCtrl.UpdateSettings)))

	// Admin: invitations (admin only).
	api.admin("POST /admin/invitations", need(access.W(access.Admin)), csrf(http.HandlerFunc(invCtrl.Create)))
	api.admin("GET /admin/invitations", need(access.R(access.Admin)), http.HandlerFunc(invCtrl.List))
	api.admin("DELETE /admin/invitations/{id}", need(access.W(access.Admin)), csrf(http.HandlerFunc(invCtrl.Delete)))

	// Admin: audit trail across products, including deleted ones (admin only).
	api.admin("GET /admin/audit", need(access.R(access.Admin)), http.HandlerFunc(auditCtrl.AdminList))
	api.admin("GET /admin/audit/export", need(access.R(access.Admin)), http.HandlerFunc(auditCtrl.AdminExport))
	api.admin("GET /admin/audit/deleted-products", need(access.R(access.Admin)), http.HandlerFunc(auditCtrl.AdminDeletedProducts))
	api.admin("DELETE /admin/audit/products/{productId}", need(access.W(access.Admin)), csrf(http.HandlerFunc(auditCtrl.AdminPurge)))

	// Products.
	api.authed("GET /products", noScope, http.HandlerFunc(productCtrl.List))
	api.authed("POST /products", need(access.W(access.Settings)), csrf(http.HandlerFunc(productCtrl.Create)))
	api.authed("GET /products/{productId}", noScope, http.HandlerFunc(productCtrl.Get))
	api.authed("PATCH /products/{productId}", need(access.W(access.Settings)), csrf(http.HandlerFunc(productCtrl.Update)))
	api.authed("DELETE /products/{productId}", need(access.W(access.Settings)), csrf(http.HandlerFunc(productCtrl.Delete)))

	// Product members.
	api.authed("GET /products/{productId}/members", need(access.R(access.Settings)), http.HandlerFunc(productCtrl.ListMembers))
	api.authed("POST /products/{productId}/members", need(access.W(access.Settings)), csrf(http.HandlerFunc(productCtrl.AddMember)))
	api.authed("PATCH /products/{productId}/members/{userId}", need(access.W(access.Settings)), csrf(http.HandlerFunc(productCtrl.UpdateMember)))
	api.authed("DELETE /products/{productId}/members/{userId}", need(access.W(access.Settings)), csrf(http.HandlerFunc(productCtrl.RemoveMember)))
	api.authed("POST /products/{productId}/transfer-ownership", need(access.W(access.Settings)), csrf(http.HandlerFunc(productCtrl.TransferOwnership)))
	api.authed("POST /products/{productId}/invitations", need(access.W(access.Settings)), csrf(http.HandlerFunc(invCtrl.CreateForProduct)))

	// Components.
	api.authed("GET /products/{productId}/components", need(access.R(access.Structure)), http.HandlerFunc(componentCtrl.ListComponents))
	api.authed("POST /products/{productId}/components", need(access.W(access.Structure)), csrf(http.HandlerFunc(componentCtrl.CreateComponent)))
	api.authed("GET /products/{productId}/components/{id}", need(access.R(access.Structure)), http.HandlerFunc(componentCtrl.GetComponent))
	api.authed("PATCH /products/{productId}/components/{id}", need(access.W(access.Structure)), csrf(http.HandlerFunc(componentCtrl.UpdateComponent)))
	api.authed("DELETE /products/{productId}/components/{id}", need(access.W(access.Structure)), csrf(http.HandlerFunc(componentCtrl.DeleteComponent)))

	// Capabilities.
	api.authed("GET /products/{productId}/capabilities", need(access.R(access.Structure)), http.HandlerFunc(componentCtrl.ListCapabilities))
	api.authed("POST /products/{productId}/capabilities", need(access.W(access.Structure)), csrf(http.HandlerFunc(componentCtrl.CreateCapability)))
	api.authed("GET /products/{productId}/capabilities/{id}", need(access.R(access.Structure)), http.HandlerFunc(componentCtrl.GetCapability))
	api.authed("PATCH /products/{productId}/capabilities/{id}", need(access.W(access.Structure)), csrf(http.HandlerFunc(componentCtrl.UpdateCapability)))
	api.authed("DELETE /products/{productId}/capabilities/{id}", need(access.W(access.Structure)), csrf(http.HandlerFunc(componentCtrl.DeleteCapability)))

	// Capability-requirement links.
	api.authed("GET /products/{productId}/capabilities/{id}/requirements", need(access.R(access.Structure), access.R(access.Backlog)), http.HandlerFunc(componentCtrl.ListCapabilityRequirements))
	api.authed("POST /products/{productId}/capabilities/{id}/requirements", need(access.W(access.Structure), access.R(access.Backlog)), csrf(http.HandlerFunc(componentCtrl.LinkCapabilityRequirement)))
	api.authed("DELETE /products/{productId}/capabilities/{id}/requirements/{reqId}", need(access.W(access.Structure), access.R(access.Backlog)), reqParamRef(csrf(http.HandlerFunc(componentCtrl.UnlinkCapabilityRequirement))))
	api.authed("GET /products/{productId}/capabilities/{id}/test-cases", need(access.R(access.Structure), access.R(access.Tests)), http.HandlerFunc(testingCtrl.ListCapabilityTestCases))
	api.authed("GET /products/{productId}/capabilities/{id}/defects", need(access.R(access.Structure), access.R(access.Defects)), backlogCtrl.LinkedDefects(model.DefectLinkTargetCapability))
	api.authed("GET /products/{productId}/capabilities/{id}/constraints", need(access.R(access.Structure), access.R(access.Backlog)), http.HandlerFunc(constraintCtrl.ListCapabilityConstraints))

	// Requirement-capability links.
	api.authed("GET /products/{productId}/requirements/{id}/capabilities", need(access.R(access.Backlog), access.R(access.Structure)), reqRef(http.HandlerFunc(componentCtrl.ListRequirementCapabilities)))
	api.authed("POST /products/{productId}/requirements/{id}/capabilities", need(access.W(access.Backlog), access.R(access.Structure)), reqRef(csrf(http.HandlerFunc(componentCtrl.LinkRequirementCapability))))
	api.authed("DELETE /products/{productId}/requirements/{id}/capabilities/{capId}", need(access.W(access.Backlog), access.R(access.Structure)), reqRef(csrf(http.HandlerFunc(componentCtrl.UnlinkRequirementCapability))))

	// Milestones.
	api.authed("GET /products/{productId}/milestones", need(access.R(access.Structure)), http.HandlerFunc(planningCtrl.ListMilestones))
	api.authed("POST /products/{productId}/milestones", need(access.W(access.Structure)), csrf(http.HandlerFunc(planningCtrl.CreateMilestone)))
	api.authed("GET /products/{productId}/milestones/{id}", need(access.R(access.Structure)), http.HandlerFunc(planningCtrl.GetMilestone))
	api.authed("PATCH /products/{productId}/milestones/{id}", need(access.W(access.Structure)), csrf(http.HandlerFunc(planningCtrl.UpdateMilestone)))
	api.authed("DELETE /products/{productId}/milestones/{id}", need(access.W(access.Structure)), csrf(http.HandlerFunc(planningCtrl.DeleteMilestone)))

	// Sprints.
	api.authed("GET /products/{productId}/sprints", need(access.R(access.Structure)), http.HandlerFunc(planningCtrl.ListSprints))
	api.authed("POST /products/{productId}/sprints", need(access.W(access.Structure)), csrf(http.HandlerFunc(planningCtrl.CreateSprint)))
	api.authed("GET /products/{productId}/sprints/{id}", need(access.R(access.Structure)), http.HandlerFunc(planningCtrl.GetSprint))
	api.authed("PATCH /products/{productId}/sprints/{id}", need(access.W(access.Structure)), csrf(http.HandlerFunc(planningCtrl.UpdateSprint)))
	api.authed("DELETE /products/{productId}/sprints/{id}", need(access.W(access.Structure)), csrf(http.HandlerFunc(planningCtrl.DeleteSprint)))

	// Statuses.
	api.authed("GET /products/{productId}/statuses", need(access.R(access.Settings)), http.HandlerFunc(planningCtrl.ListStatuses))
	api.authed("POST /products/{productId}/statuses", need(access.W(access.Settings)), csrf(http.HandlerFunc(planningCtrl.CreateStatus)))
	api.authed("GET /products/{productId}/statuses/{id}", need(access.R(access.Settings)), http.HandlerFunc(planningCtrl.GetStatus))
	api.authed("POST /products/{productId}/statuses/split", need(access.W(access.Settings)), csrf(http.HandlerFunc(productCtrl.SplitStatuses)))
	api.authed("PATCH /products/{productId}/statuses/{id}", need(access.W(access.Settings)), csrf(http.HandlerFunc(planningCtrl.UpdateStatus)))
	api.authed("DELETE /products/{productId}/statuses/{id}", need(access.W(access.Settings)), csrf(http.HandlerFunc(planningCtrl.DeleteStatus)))

	// Pyramid levels.
	api.authed("GET /products/{productId}/pyramid-levels", need(access.R(access.Settings)), http.HandlerFunc(planningCtrl.ListPyramidLevels))
	api.authed("POST /products/{productId}/pyramid-levels", need(access.W(access.Settings)), csrf(http.HandlerFunc(planningCtrl.CreatePyramidLevel)))
	api.authed("GET /products/{productId}/pyramid-levels/{id}", need(access.R(access.Settings)), http.HandlerFunc(planningCtrl.GetPyramidLevel))
	api.authed("PATCH /products/{productId}/pyramid-levels/{id}", need(access.W(access.Settings)), csrf(http.HandlerFunc(planningCtrl.UpdatePyramidLevel)))
	api.authed("DELETE /products/{productId}/pyramid-levels/{id}", need(access.W(access.Settings)), csrf(http.HandlerFunc(planningCtrl.DeletePyramidLevel)))

	// Runtime environments.
	api.authed("GET /products/{productId}/runtime-environments", need(access.R(access.Settings)), http.HandlerFunc(planningCtrl.ListRuntimeEnvironments))
	api.authed("POST /products/{productId}/runtime-environments", need(access.W(access.Settings)), csrf(http.HandlerFunc(planningCtrl.CreateRuntimeEnvironment)))
	api.authed("DELETE /products/{productId}/runtime-environments/{id}", need(access.W(access.Settings)), csrf(http.HandlerFunc(planningCtrl.DeleteRuntimeEnvironment)))

	// Categories.
	api.authed("GET /products/{productId}/categories", need(access.R(access.Settings)), http.HandlerFunc(categoryCtrl.ListCategories))
	api.authed("POST /products/{productId}/categories", need(access.W(access.Settings)), csrf(http.HandlerFunc(categoryCtrl.CreateCategory)))
	api.authed("GET /products/{productId}/categories/{id}", need(access.R(access.Settings)), http.HandlerFunc(categoryCtrl.GetCategory))
	api.authed("PATCH /products/{productId}/categories/{id}", need(access.W(access.Settings)), csrf(http.HandlerFunc(categoryCtrl.UpdateCategory)))
	api.authed("DELETE /products/{productId}/categories/{id}", need(access.W(access.Settings)), csrf(http.HandlerFunc(categoryCtrl.DeleteCategory)))

	// Requirements.
	api.authed("GET /products/{productId}/requirements", need(access.R(access.Backlog)), http.HandlerFunc(backlogCtrl.ListRequirements))
	api.authed("POST /products/{productId}/requirements", need(access.W(access.Backlog)), csrf(http.HandlerFunc(backlogCtrl.CreateRequirement)))
	api.authed("GET /products/{productId}/requirements/{id}", need(access.R(access.Backlog)), reqRef(http.HandlerFunc(backlogCtrl.GetRequirement)))
	api.authed("PATCH /products/{productId}/requirements/{id}", need(access.W(access.Backlog)), reqRef(csrf(http.HandlerFunc(backlogCtrl.UpdateRequirement))))
	api.authed("DELETE /products/{productId}/requirements/{id}", need(access.W(access.Backlog)), reqRef(csrf(http.HandlerFunc(backlogCtrl.DeleteRequirement))))
	api.authed("GET /products/{productId}/requirements/{id}/children", need(access.R(access.Backlog)), reqRef(http.HandlerFunc(backlogCtrl.ListChildRequirements)))
	api.authed("GET /products/{productId}/requirements/{id}/assignees", need(access.R(access.Backlog)), reqRef(http.HandlerFunc(backlogCtrl.ListRequirementAssignees)))
	api.authed("POST /products/{productId}/requirements/{id}/assignees", need(access.W(access.Backlog)), reqRef(csrf(http.HandlerFunc(backlogCtrl.AssignRequirement))))
	api.authed("DELETE /products/{productId}/requirements/{id}/assignees/{userId}", need(access.W(access.Backlog)), reqRef(csrf(http.HandlerFunc(backlogCtrl.UnassignRequirement))))
	api.authed("GET /products/{productId}/requirements/{id}/test-cases", need(access.R(access.Backlog), access.R(access.Tests)), reqRef(http.HandlerFunc(testingCtrl.ListRequirementTestCases)))
	api.authed("GET /products/{productId}/requirements/{id}/defects", need(access.R(access.Backlog), access.R(access.Defects)), reqRef(backlogCtrl.LinkedDefects(model.DefectLinkTargetRequirement)))

	// Constraints.
	api.authed("GET /products/{productId}/constraints", need(access.R(access.Backlog)), http.HandlerFunc(constraintCtrl.ListConstraints))
	api.authed("POST /products/{productId}/constraints", need(access.W(access.Backlog)), csrf(http.HandlerFunc(constraintCtrl.CreateConstraint)))
	api.authed("GET /products/{productId}/constraints/{id}", need(access.R(access.Backlog)), conRef(http.HandlerFunc(constraintCtrl.GetConstraint)))
	api.authed("PATCH /products/{productId}/constraints/{id}", need(access.W(access.Backlog)), conRef(csrf(http.HandlerFunc(constraintCtrl.UpdateConstraint))))
	api.authed("DELETE /products/{productId}/constraints/{id}", need(access.W(access.Backlog)), conRef(csrf(http.HandlerFunc(constraintCtrl.DeleteConstraint))))
	api.authed("GET /products/{productId}/constraints/{id}/capabilities", need(access.R(access.Backlog), access.R(access.Structure)), conRef(http.HandlerFunc(constraintCtrl.ListConstraintCapabilities)))
	api.authed("POST /products/{productId}/constraints/{id}/capabilities", need(access.W(access.Backlog), access.R(access.Structure)), conRef(csrf(http.HandlerFunc(constraintCtrl.LinkConstraintCapability))))
	api.authed("DELETE /products/{productId}/constraints/{id}/capabilities/{capId}", need(access.W(access.Backlog), access.R(access.Structure)), conRef(csrf(http.HandlerFunc(constraintCtrl.UnlinkConstraintCapability))))
	api.authed("GET /products/{productId}/constraints/{id}/test-cases", need(access.R(access.Backlog), access.R(access.Tests)), conRef(http.HandlerFunc(testingCtrl.ListConstraintTestCases)))

	// Defects.
	api.authed("GET /products/{productId}/defects", need(access.R(access.Defects)), http.HandlerFunc(backlogCtrl.ListDefects))
	api.authed("POST /products/{productId}/defects", need(access.W(access.Defects)), csrf(http.HandlerFunc(backlogCtrl.CreateDefect)))
	api.authed("GET /products/{productId}/defects/{id}", need(access.R(access.Defects)), defRef(http.HandlerFunc(backlogCtrl.GetDefect)))
	api.authed("PATCH /products/{productId}/defects/{id}", need(access.W(access.Defects)), defRef(csrf(http.HandlerFunc(backlogCtrl.UpdateDefect))))
	api.authed("DELETE /products/{productId}/defects/{id}", need(access.W(access.Defects)), defRef(csrf(http.HandlerFunc(backlogCtrl.DeleteDefect))))
	api.authed("GET /products/{productId}/defects/{id}/assignees", need(access.R(access.Defects)), defRef(http.HandlerFunc(backlogCtrl.ListDefectAssignees)))
	api.authed("POST /products/{productId}/defects/{id}/assignees", need(access.W(access.Defects)), defRef(csrf(http.HandlerFunc(backlogCtrl.AssignDefect))))
	api.authed("DELETE /products/{productId}/defects/{id}/assignees/{userId}", need(access.W(access.Defects)), defRef(csrf(http.HandlerFunc(backlogCtrl.UnassignDefect))))
	api.authed("GET /products/{productId}/defects/{id}/links", need(access.R(access.Defects)), defRef(http.HandlerFunc(backlogCtrl.ListDefectLinks)))
	api.authed("POST /products/{productId}/defects/{id}/links", need(access.W(access.Defects)), defRef(csrf(http.HandlerFunc(backlogCtrl.AddDefectLink))))
	api.authed("DELETE /products/{productId}/defects/{id}/links/{linkId}", need(access.W(access.Defects)), defRef(csrf(http.HandlerFunc(backlogCtrl.RemoveDefectLink))))

	// Test cases.
	api.authed("GET /products/{productId}/test-cases", need(access.R(access.Tests)), http.HandlerFunc(testingCtrl.ListTestCases))
	api.authed("POST /products/{productId}/test-cases", need(access.W(access.Tests)), csrf(http.HandlerFunc(testingCtrl.CreateTestCase)))
	api.authed("GET /products/{productId}/test-cases/{id}", need(access.R(access.Tests)), http.HandlerFunc(testingCtrl.GetTestCase))
	api.authed("PATCH /products/{productId}/test-cases/{id}", need(access.W(access.Tests)), csrf(http.HandlerFunc(testingCtrl.UpdateTestCase)))
	api.authed("DELETE /products/{productId}/test-cases/{id}", need(access.W(access.Tests)), csrf(http.HandlerFunc(testingCtrl.DeleteTestCase)))
	api.authed("GET /products/{productId}/test-cases/{id}/requirements", need(access.R(access.Tests), access.R(access.Backlog)), http.HandlerFunc(testingCtrl.ListTestCaseRequirements))
	api.authed("POST /products/{productId}/test-cases/{id}/requirements", need(access.W(access.Tests), access.R(access.Backlog)), csrf(http.HandlerFunc(testingCtrl.LinkRequirement)))
	api.authed("DELETE /products/{productId}/test-cases/{id}/requirements/{reqId}", need(access.W(access.Tests), access.R(access.Backlog)), reqParamRef(csrf(http.HandlerFunc(testingCtrl.UnlinkRequirement))))
	api.authed("GET /products/{productId}/test-cases/{id}/constraints", need(access.R(access.Tests), access.R(access.Backlog)), http.HandlerFunc(testingCtrl.ListTestCaseConstraints))
	api.authed("POST /products/{productId}/test-cases/{id}/constraints", need(access.W(access.Tests), access.R(access.Backlog)), csrf(http.HandlerFunc(testingCtrl.LinkConstraint)))
	api.authed("DELETE /products/{productId}/test-cases/{id}/constraints/{conId}", need(access.W(access.Tests), access.R(access.Backlog)), conParamRef(csrf(http.HandlerFunc(testingCtrl.UnlinkConstraint))))
	api.authed("GET /products/{productId}/test-cases/{id}/environments", need(access.R(access.Tests)), http.HandlerFunc(testingCtrl.ListTestCaseEnvironments))
	api.authed("POST /products/{productId}/test-cases/{id}/environments", need(access.W(access.Tests)), csrf(http.HandlerFunc(testingCtrl.AddEnvironment)))
	api.authed("DELETE /products/{productId}/test-cases/{id}/environments/{envId}", need(access.W(access.Tests)), csrf(http.HandlerFunc(testingCtrl.RemoveEnvironment)))
	api.authed("GET /products/{productId}/test-cases/{id}/results", need(access.R(access.Tests)), http.HandlerFunc(testingCtrl.ListTestCaseResults))
	api.authed("GET /products/{productId}/test-cases/{id}/defects", need(access.R(access.Tests), access.R(access.Defects)), backlogCtrl.LinkedDefects(model.DefectLinkTargetTestCase))

	// Campaigns.
	api.authed("GET /products/{productId}/campaigns", need(access.R(access.Tests)), http.HandlerFunc(testingCtrl.ListCampaigns))
	api.authed("POST /products/{productId}/campaigns", need(access.W(access.Tests)), csrf(http.HandlerFunc(testingCtrl.CreateCampaign)))
	api.authed("GET /products/{productId}/campaigns/{id}", need(access.R(access.Tests)), http.HandlerFunc(testingCtrl.GetCampaign))
	api.authed("PATCH /products/{productId}/campaigns/{id}", need(access.W(access.Tests)), csrf(http.HandlerFunc(testingCtrl.UpdateCampaign)))
	api.authed("DELETE /products/{productId}/campaigns/{id}", need(access.W(access.Tests)), csrf(http.HandlerFunc(testingCtrl.DeleteCampaign)))
	api.authed("POST /products/{productId}/campaigns/{id}/activate", need(access.W(access.Tests)), csrf(http.HandlerFunc(testingCtrl.ActivateCampaign)))
	api.authed("POST /products/{productId}/campaigns/{id}/close", need(access.W(access.Tests)), csrf(http.HandlerFunc(testingCtrl.CloseCampaign)))
	api.authed("POST /products/{productId}/campaigns/{id}/test-cases", need(access.W(access.Tests)), csrf(http.HandlerFunc(testingCtrl.AddCampaignTestCases)))
	api.authed("DELETE /products/{productId}/campaigns/{id}/test-cases/{testCaseId}", need(access.W(access.Tests)), csrf(http.HandlerFunc(testingCtrl.RemoveCampaignTestCase)))
	api.authed("GET /products/{productId}/campaigns/{id}/progress", need(access.R(access.Tests)), http.HandlerFunc(testingCtrl.Progress))
	api.authed("GET /products/{productId}/campaigns/{id}/events", need(access.R(access.Tests)), http.HandlerFunc(testingCtrl.ListResultEvents))
	api.authed("GET /products/{productId}/campaigns/{id}/results", need(access.R(access.Tests)), http.HandlerFunc(testingCtrl.ListCampaignResults))
	api.authed("PATCH /products/{productId}/campaigns/{id}/results/{resultId}", need(access.W(access.Tests)), csrf(http.HandlerFunc(testingCtrl.UpdateResult)))

	// Automated test runs (FR-118): CI uploads results into the active campaign.
	api.authed("POST /products/{productId}/test-runs", need(access.W(access.Tests)), csrf(http.HandlerFunc(testRunCtrl.Ingest)))

	// Comments.
	api.authed("GET /products/{productId}/comments", need(access.R(access.Discussion)), http.HandlerFunc(discussionCtrl.ListComments))
	api.authed("POST /products/{productId}/comments", need(access.W(access.Discussion)), csrf(http.HandlerFunc(discussionCtrl.CreateComment)))
	api.authed("GET /products/{productId}/comments/{id}", need(access.R(access.Discussion)), http.HandlerFunc(discussionCtrl.GetComment))
	api.authed("PATCH /products/{productId}/comments/{id}", need(access.W(access.Discussion)), csrf(http.HandlerFunc(discussionCtrl.UpdateComment)))
	api.authed("DELETE /products/{productId}/comments/{id}", need(access.W(access.Discussion)), csrf(http.HandlerFunc(discussionCtrl.DeleteComment)))

	// Attachments.
	api.authed("GET /products/{productId}/attachments", need(access.R(access.Discussion)), http.HandlerFunc(discussionCtrl.ListAttachments))
	api.authed("POST /products/{productId}/attachments", need(access.W(access.Discussion)), csrf(http.HandlerFunc(discussionCtrl.CreateAttachment)))
	api.authed("GET /products/{productId}/attachments/{id}", need(access.R(access.Discussion)), http.HandlerFunc(discussionCtrl.GetAttachment))
	api.authed("GET /products/{productId}/attachments/{id}/download", need(access.R(access.Discussion)), http.HandlerFunc(discussionCtrl.DownloadAttachment))
	api.authed("DELETE /products/{productId}/attachments/{id}", need(access.W(access.Discussion)), csrf(http.HandlerFunc(discussionCtrl.DeleteAttachment)))

	// Documents (FR-50..FR-58): their own scope (SC-54); links need read on
	// the scope of the linked item as well (SC-55).
	api.authed("GET /products/{productId}/documents", need(access.R(access.Documents)), http.HandlerFunc(documentCtrl.ListDocuments))
	api.authed("POST /products/{productId}/documents", need(access.W(access.Documents)), csrf(http.HandlerFunc(documentCtrl.CreateDocument)))
	api.authed("GET /products/{productId}/documents/{id}", need(access.R(access.Documents)), docRef(http.HandlerFunc(documentCtrl.GetDocument)))
	api.authed("PATCH /products/{productId}/documents/{id}", need(access.W(access.Documents)), docRef(csrf(http.HandlerFunc(documentCtrl.UpdateDocument))))
	api.authed("DELETE /products/{productId}/documents/{id}", need(access.W(access.Documents)), docRef(csrf(http.HandlerFunc(documentCtrl.DeleteDocument))))
	api.authed("GET /products/{productId}/documents/{id}/raw", need(access.R(access.Documents)), docRef(http.HandlerFunc(documentCtrl.Raw)))
	api.authed("GET /products/{productId}/documents/{id}/revisions", need(access.R(access.Documents)), docRef(http.HandlerFunc(documentCtrl.ListRevisions)))
	api.authed("GET /products/{productId}/documents/{id}/revisions/{rev}", need(access.R(access.Documents)), docRef(http.HandlerFunc(documentCtrl.GetRevision)))
	api.authed("GET /products/{productId}/documents/{id}/diff", need(access.R(access.Documents)), docRef(http.HandlerFunc(documentCtrl.Diff)))
	api.authed("POST /products/{productId}/documents/{id}/restore", need(access.W(access.Documents)), docRef(csrf(http.HandlerFunc(documentCtrl.Restore))))
	api.authed("GET /products/{productId}/documents/{id}/requirements", need(access.R(access.Documents), access.R(access.Backlog)), docRef(documentCtrl.ListLinked(model.DocumentLinkRequirement)))
	api.authed("POST /products/{productId}/documents/{id}/requirements", need(access.W(access.Documents), access.R(access.Backlog)), docRef(csrf(documentCtrl.Link(model.DocumentLinkRequirement, svc.Backlog.ResolveRequirementRef))))
	api.authed("DELETE /products/{productId}/documents/{id}/requirements/{targetId}", need(access.W(access.Documents), access.R(access.Backlog)), docRef(docTargetReqRef(csrf(documentCtrl.Unlink(model.DocumentLinkRequirement)))))
	api.authed("GET /products/{productId}/documents/{id}/constraints", need(access.R(access.Documents), access.R(access.Backlog)), docRef(documentCtrl.ListLinked(model.DocumentLinkConstraint)))
	api.authed("POST /products/{productId}/documents/{id}/constraints", need(access.W(access.Documents), access.R(access.Backlog)), docRef(csrf(documentCtrl.Link(model.DocumentLinkConstraint, svc.Constraint.ResolveConstraintRef))))
	api.authed("DELETE /products/{productId}/documents/{id}/constraints/{targetId}", need(access.W(access.Documents), access.R(access.Backlog)), docRef(docTargetConRef(csrf(documentCtrl.Unlink(model.DocumentLinkConstraint)))))
	api.authed("GET /products/{productId}/documents/{id}/test-cases", need(access.R(access.Documents), access.R(access.Tests)), docRef(documentCtrl.ListLinked(model.DocumentLinkTestCase)))
	api.authed("POST /products/{productId}/documents/{id}/test-cases", need(access.W(access.Documents), access.R(access.Tests)), docRef(csrf(documentCtrl.Link(model.DocumentLinkTestCase, nil))))
	api.authed("DELETE /products/{productId}/documents/{id}/test-cases/{targetId}", need(access.W(access.Documents), access.R(access.Tests)), docRef(csrf(documentCtrl.Unlink(model.DocumentLinkTestCase))))
	api.authed("GET /products/{productId}/requirements/{id}/documents", need(access.R(access.Backlog), access.R(access.Documents)), reqRef(documentCtrl.DocumentsOf(model.DocumentLinkRequirement)))
	api.authed("GET /products/{productId}/constraints/{id}/documents", need(access.R(access.Backlog), access.R(access.Documents)), conRef(documentCtrl.DocumentsOf(model.DocumentLinkConstraint)))
	api.authed("GET /products/{productId}/test-cases/{id}/documents", need(access.R(access.Tests), access.R(access.Documents)), documentCtrl.DocumentsOf(model.DocumentLinkTestCase))

	// Specification export (FR-137): the structure and the backlog, as Markdown.
	api.authed("GET /products/{productId}/specification", need(access.R(access.Structure), access.R(access.Backlog)), http.HandlerFunc(specCtrl.Export))

	// Audit trail.
	api.authed("GET /products/{productId}/audit", readAll, http.HandlerFunc(auditCtrl.List))

	mux.Handle("/api/v1/", http.StripPrefix("/api/v1", apiV1))
	return api.needs
}

// apiRoutes registers authenticated API routes together with the token scopes
// they need (SC-54, SC-55). Every authenticated route declares its scopes,
// so a route cannot be added without deciding what a token needs for it.
//
// The rule: every resource named in the path needs read, and a state-changing
// method needs write on the first resource of the path. TestAPIRouteScopes
// checks every route against it.
type apiRoutes struct {
	mux   *http.ServeMux
	needs map[string][]access.Need
}

// authed registers a route that requires authentication; a token must hold
// every scope in need.
func (a *apiRoutes) authed(pattern string, need []access.Need, h http.Handler) {
	a.needs[pattern] = need
	a.mux.Handle(pattern, middleware.RequireAuth(middleware.RequireScopes(need...)(h)))
}

// admin registers a route that requires an administrator; a token must hold
// every scope in need (always the admin scope, SC-56).
func (a *apiRoutes) admin(pattern string, need []access.Need, h http.Handler) {
	a.needs[pattern] = need
	a.mux.Handle(pattern, middleware.RequireAuth(middleware.RequireScopes(need...)(middleware.RequireAdmin(h))))
}

// need lists the scopes a route needs.
func need(n ...access.Need) []access.Need { return n }

var (
	// noScope marks routes any authenticated token may call.
	noScope = need()
	// readAll needs read on every product scope, for views across all of a
	// product's data (the audit trail).
	readAll = need(access.R(access.Structure), access.R(access.Backlog), access.R(access.Documents), access.R(access.Defects),
		access.R(access.Tests), access.R(access.Discussion), access.R(access.Settings))
)

// csrf wraps a handler with CSRF middleware.
func csrf(h http.Handler) http.Handler {
	return middleware.CSRF(h)
}
