package router

import (
	"net/http"

	"nexmap.net/internal/controller"
	"nexmap.net/internal/middleware"
	"nexmap.net/internal/service"
)

func mountAPIv1(mux *http.ServeMux, svc *service.Services, cookieSecure bool) {
	apiV1 := http.NewServeMux()

	authCtrl := controller.NewAuth(svc.Auth, svc.User, cookieSecure)
	userCtrl := controller.NewUser(svc.User)
	patCtrl := controller.NewPAT(svc.PAT)
	adminCtrl := controller.NewAdmin(svc.User, svc.Setting)
	invCtrl := controller.NewInvitation(svc.Invitation)
	productCtrl := controller.NewProduct(svc.Product)
	componentCtrl := controller.NewComponent(svc.Component)
	planningCtrl := controller.NewPlanning(svc.Milestone)
	categoryCtrl := controller.NewCategory(svc.Category)
	backlogCtrl := controller.NewBacklog(svc.Backlog)
	constraintCtrl := controller.NewConstraint(svc.Constraint)
	testingCtrl := controller.NewTesting(svc.Testing)
	discussionCtrl := controller.NewDiscussion(svc.Discussion)
	auditCtrl := controller.NewAudit(svc.Audit)

	// Health (public).
	apiV1.HandleFunc("GET /health", controller.Health)

	// Auth (public).
	apiV1.HandleFunc("POST /auth/login", authCtrl.Login)
	apiV1.HandleFunc("POST /auth/register", authCtrl.Register)
	apiV1.HandleFunc("POST /auth/accept-invite", authCtrl.AcceptInvite)

	// Auth (authenticated).
	apiV1.Handle("POST /auth/logout", authed(csrf(http.HandlerFunc(authCtrl.Logout))))

	// Current user (authenticated).
	apiV1.Handle("GET /user/me", authed(http.HandlerFunc(userCtrl.GetMe)))
	apiV1.Handle("PATCH /user/me", authed(csrf(http.HandlerFunc(userCtrl.UpdateMe))))
	apiV1.Handle("POST /user/me/password", authed(csrf(http.HandlerFunc(userCtrl.ChangePassword))))
	apiV1.Handle("POST /user/me/delete", authed(csrf(http.HandlerFunc(userCtrl.RequestDeletion))))

	// Personal access tokens (authenticated).
	apiV1.Handle("GET /user/me/tokens", authed(http.HandlerFunc(patCtrl.List)))
	apiV1.Handle("POST /user/me/tokens", authed(csrf(http.HandlerFunc(patCtrl.Create))))
	apiV1.Handle("DELETE /user/me/tokens/{id}", authed(csrf(http.HandlerFunc(patCtrl.Revoke))))

	// Admin: users (admin only).
	apiV1.Handle("GET /admin/users", admin(http.HandlerFunc(adminCtrl.ListUsers)))
	apiV1.Handle("GET /admin/users/{id}", admin(http.HandlerFunc(adminCtrl.GetUser)))
	apiV1.Handle("POST /admin/users/{id}/lock", admin(csrf(http.HandlerFunc(adminCtrl.LockUser))))
	apiV1.Handle("POST /admin/users/{id}/unlock", admin(csrf(http.HandlerFunc(adminCtrl.UnlockUser))))
	apiV1.Handle("POST /admin/users/{id}/password", admin(csrf(http.HandlerFunc(adminCtrl.SetPassword))))
	apiV1.Handle("POST /admin/users/{id}/approve", admin(csrf(http.HandlerFunc(adminCtrl.ApproveUser))))
	apiV1.Handle("DELETE /admin/users/{id}", admin(csrf(http.HandlerFunc(adminCtrl.DeleteUser))))

	// Admin: settings (admin only).
	apiV1.Handle("GET /admin/settings", admin(http.HandlerFunc(adminCtrl.GetSettings)))
	apiV1.Handle("PATCH /admin/settings", admin(csrf(http.HandlerFunc(adminCtrl.UpdateSettings))))

	// Admin: invitations (admin only).
	apiV1.Handle("POST /admin/invitations", admin(csrf(http.HandlerFunc(invCtrl.Create))))
	apiV1.Handle("GET /admin/invitations", admin(http.HandlerFunc(invCtrl.List)))
	apiV1.Handle("DELETE /admin/invitations/{id}", admin(csrf(http.HandlerFunc(invCtrl.Delete))))

	// Products.
	apiV1.Handle("GET /products", authed(http.HandlerFunc(productCtrl.List)))
	apiV1.Handle("POST /products", authed(csrf(http.HandlerFunc(productCtrl.Create))))
	apiV1.Handle("GET /products/{productId}", authed(http.HandlerFunc(productCtrl.Get)))
	apiV1.Handle("PATCH /products/{productId}", authed(csrf(http.HandlerFunc(productCtrl.Update))))
	apiV1.Handle("DELETE /products/{productId}", authed(csrf(http.HandlerFunc(productCtrl.Delete))))

	// Product members.
	apiV1.Handle("GET /products/{productId}/members", authed(http.HandlerFunc(productCtrl.ListMembers)))
	apiV1.Handle("POST /products/{productId}/members", authed(csrf(http.HandlerFunc(productCtrl.AddMember))))
	apiV1.Handle("PATCH /products/{productId}/members/{userId}", authed(csrf(http.HandlerFunc(productCtrl.UpdateMember))))
	apiV1.Handle("DELETE /products/{productId}/members/{userId}", authed(csrf(http.HandlerFunc(productCtrl.RemoveMember))))
	apiV1.Handle("POST /products/{productId}/transfer-ownership", authed(csrf(http.HandlerFunc(productCtrl.TransferOwnership))))

	// Components.
	apiV1.Handle("GET /products/{productId}/components", authed(http.HandlerFunc(componentCtrl.ListComponents)))
	apiV1.Handle("POST /products/{productId}/components", authed(csrf(http.HandlerFunc(componentCtrl.CreateComponent))))
	apiV1.Handle("GET /products/{productId}/components/{id}", authed(http.HandlerFunc(componentCtrl.GetComponent)))
	apiV1.Handle("PATCH /products/{productId}/components/{id}", authed(csrf(http.HandlerFunc(componentCtrl.UpdateComponent))))
	apiV1.Handle("DELETE /products/{productId}/components/{id}", authed(csrf(http.HandlerFunc(componentCtrl.DeleteComponent))))

	// Capabilities.
	apiV1.Handle("GET /products/{productId}/capabilities", authed(http.HandlerFunc(componentCtrl.ListCapabilities)))
	apiV1.Handle("POST /products/{productId}/capabilities", authed(csrf(http.HandlerFunc(componentCtrl.CreateCapability))))
	apiV1.Handle("GET /products/{productId}/capabilities/{id}", authed(http.HandlerFunc(componentCtrl.GetCapability)))
	apiV1.Handle("PATCH /products/{productId}/capabilities/{id}", authed(csrf(http.HandlerFunc(componentCtrl.UpdateCapability))))
	apiV1.Handle("DELETE /products/{productId}/capabilities/{id}", authed(csrf(http.HandlerFunc(componentCtrl.DeleteCapability))))

	// Capability-requirement links.
	apiV1.Handle("GET /products/{productId}/capabilities/{id}/requirements", authed(http.HandlerFunc(componentCtrl.ListCapabilityRequirements)))
	apiV1.Handle("POST /products/{productId}/capabilities/{id}/requirements", authed(csrf(http.HandlerFunc(componentCtrl.LinkCapabilityRequirement))))
	apiV1.Handle("DELETE /products/{productId}/capabilities/{id}/requirements/{reqId}", authed(csrf(http.HandlerFunc(componentCtrl.UnlinkCapabilityRequirement))))

	// Requirement-capability links.
	apiV1.Handle("GET /products/{productId}/requirements/{id}/capabilities", authed(http.HandlerFunc(componentCtrl.ListRequirementCapabilities)))
	apiV1.Handle("POST /products/{productId}/requirements/{id}/capabilities", authed(csrf(http.HandlerFunc(componentCtrl.LinkRequirementCapability))))
	apiV1.Handle("DELETE /products/{productId}/requirements/{id}/capabilities/{capId}", authed(csrf(http.HandlerFunc(componentCtrl.UnlinkRequirementCapability))))

	// Milestones.
	apiV1.Handle("GET /products/{productId}/milestones", authed(http.HandlerFunc(planningCtrl.ListMilestones)))
	apiV1.Handle("POST /products/{productId}/milestones", authed(csrf(http.HandlerFunc(planningCtrl.CreateMilestone))))
	apiV1.Handle("GET /products/{productId}/milestones/{id}", authed(http.HandlerFunc(planningCtrl.GetMilestone)))
	apiV1.Handle("PATCH /products/{productId}/milestones/{id}", authed(csrf(http.HandlerFunc(planningCtrl.UpdateMilestone))))
	apiV1.Handle("DELETE /products/{productId}/milestones/{id}", authed(csrf(http.HandlerFunc(planningCtrl.DeleteMilestone))))

	// Sprints.
	apiV1.Handle("GET /products/{productId}/sprints", authed(http.HandlerFunc(planningCtrl.ListSprints)))
	apiV1.Handle("POST /products/{productId}/sprints", authed(csrf(http.HandlerFunc(planningCtrl.CreateSprint))))
	apiV1.Handle("GET /products/{productId}/sprints/{id}", authed(http.HandlerFunc(planningCtrl.GetSprint)))
	apiV1.Handle("PATCH /products/{productId}/sprints/{id}", authed(csrf(http.HandlerFunc(planningCtrl.UpdateSprint))))
	apiV1.Handle("DELETE /products/{productId}/sprints/{id}", authed(csrf(http.HandlerFunc(planningCtrl.DeleteSprint))))

	// Statuses.
	apiV1.Handle("GET /products/{productId}/statuses", authed(http.HandlerFunc(planningCtrl.ListStatuses)))
	apiV1.Handle("POST /products/{productId}/statuses", authed(csrf(http.HandlerFunc(planningCtrl.CreateStatus))))
	apiV1.Handle("GET /products/{productId}/statuses/{id}", authed(http.HandlerFunc(planningCtrl.GetStatus)))
	apiV1.Handle("PATCH /products/{productId}/statuses/{id}", authed(csrf(http.HandlerFunc(planningCtrl.UpdateStatus))))
	apiV1.Handle("DELETE /products/{productId}/statuses/{id}", authed(csrf(http.HandlerFunc(planningCtrl.DeleteStatus))))

	// Pyramid levels.
	apiV1.Handle("GET /products/{productId}/pyramid-levels", authed(http.HandlerFunc(planningCtrl.ListPyramidLevels)))
	apiV1.Handle("POST /products/{productId}/pyramid-levels", authed(csrf(http.HandlerFunc(planningCtrl.CreatePyramidLevel))))
	apiV1.Handle("GET /products/{productId}/pyramid-levels/{id}", authed(http.HandlerFunc(planningCtrl.GetPyramidLevel)))
	apiV1.Handle("PATCH /products/{productId}/pyramid-levels/{id}", authed(csrf(http.HandlerFunc(planningCtrl.UpdatePyramidLevel))))
	apiV1.Handle("DELETE /products/{productId}/pyramid-levels/{id}", authed(csrf(http.HandlerFunc(planningCtrl.DeletePyramidLevel))))

	// Runtime environments.
	apiV1.Handle("GET /products/{productId}/runtime-environments", authed(http.HandlerFunc(planningCtrl.ListRuntimeEnvironments)))
	apiV1.Handle("POST /products/{productId}/runtime-environments", authed(csrf(http.HandlerFunc(planningCtrl.CreateRuntimeEnvironment))))
	apiV1.Handle("DELETE /products/{productId}/runtime-environments/{id}", authed(csrf(http.HandlerFunc(planningCtrl.DeleteRuntimeEnvironment))))

	// Categories.
	apiV1.Handle("GET /products/{productId}/categories", authed(http.HandlerFunc(categoryCtrl.ListCategories)))
	apiV1.Handle("POST /products/{productId}/categories", authed(csrf(http.HandlerFunc(categoryCtrl.CreateCategory))))
	apiV1.Handle("GET /products/{productId}/categories/{id}", authed(http.HandlerFunc(categoryCtrl.GetCategory)))
	apiV1.Handle("PATCH /products/{productId}/categories/{id}", authed(csrf(http.HandlerFunc(categoryCtrl.UpdateCategory))))
	apiV1.Handle("DELETE /products/{productId}/categories/{id}", authed(csrf(http.HandlerFunc(categoryCtrl.DeleteCategory))))

	// Requirements.
	apiV1.Handle("GET /products/{productId}/requirements", authed(http.HandlerFunc(backlogCtrl.ListRequirements)))
	apiV1.Handle("POST /products/{productId}/requirements", authed(csrf(http.HandlerFunc(backlogCtrl.CreateRequirement))))
	apiV1.Handle("GET /products/{productId}/requirements/{id}", authed(http.HandlerFunc(backlogCtrl.GetRequirement)))
	apiV1.Handle("PATCH /products/{productId}/requirements/{id}", authed(csrf(http.HandlerFunc(backlogCtrl.UpdateRequirement))))
	apiV1.Handle("DELETE /products/{productId}/requirements/{id}", authed(csrf(http.HandlerFunc(backlogCtrl.DeleteRequirement))))
	apiV1.Handle("GET /products/{productId}/requirements/{id}/children", authed(http.HandlerFunc(backlogCtrl.ListChildRequirements)))
	apiV1.Handle("GET /products/{productId}/requirements/{id}/assignees", authed(http.HandlerFunc(backlogCtrl.ListRequirementAssignees)))
	apiV1.Handle("POST /products/{productId}/requirements/{id}/assignees", authed(csrf(http.HandlerFunc(backlogCtrl.AssignRequirement))))
	apiV1.Handle("DELETE /products/{productId}/requirements/{id}/assignees/{userId}", authed(csrf(http.HandlerFunc(backlogCtrl.UnassignRequirement))))

	// Constraints.
	apiV1.Handle("GET /products/{productId}/constraints", authed(http.HandlerFunc(constraintCtrl.ListConstraints)))
	apiV1.Handle("POST /products/{productId}/constraints", authed(csrf(http.HandlerFunc(constraintCtrl.CreateConstraint))))
	apiV1.Handle("GET /products/{productId}/constraints/{id}", authed(http.HandlerFunc(constraintCtrl.GetConstraint)))
	apiV1.Handle("PATCH /products/{productId}/constraints/{id}", authed(csrf(http.HandlerFunc(constraintCtrl.UpdateConstraint))))
	apiV1.Handle("DELETE /products/{productId}/constraints/{id}", authed(csrf(http.HandlerFunc(constraintCtrl.DeleteConstraint))))
	apiV1.Handle("GET /products/{productId}/constraints/{id}/capabilities", authed(http.HandlerFunc(constraintCtrl.ListConstraintCapabilities)))
	apiV1.Handle("POST /products/{productId}/constraints/{id}/capabilities", authed(csrf(http.HandlerFunc(constraintCtrl.LinkConstraintCapability))))
	apiV1.Handle("DELETE /products/{productId}/constraints/{id}/capabilities/{capId}", authed(csrf(http.HandlerFunc(constraintCtrl.UnlinkConstraintCapability))))

	// Defects.
	apiV1.Handle("GET /products/{productId}/defects", authed(http.HandlerFunc(backlogCtrl.ListDefects)))
	apiV1.Handle("POST /products/{productId}/defects", authed(csrf(http.HandlerFunc(backlogCtrl.CreateDefect))))
	apiV1.Handle("GET /products/{productId}/defects/{id}", authed(http.HandlerFunc(backlogCtrl.GetDefect)))
	apiV1.Handle("PATCH /products/{productId}/defects/{id}", authed(csrf(http.HandlerFunc(backlogCtrl.UpdateDefect))))
	apiV1.Handle("DELETE /products/{productId}/defects/{id}", authed(csrf(http.HandlerFunc(backlogCtrl.DeleteDefect))))
	apiV1.Handle("GET /products/{productId}/defects/{id}/assignees", authed(http.HandlerFunc(backlogCtrl.ListDefectAssignees)))
	apiV1.Handle("POST /products/{productId}/defects/{id}/assignees", authed(csrf(http.HandlerFunc(backlogCtrl.AssignDefect))))
	apiV1.Handle("DELETE /products/{productId}/defects/{id}/assignees/{userId}", authed(csrf(http.HandlerFunc(backlogCtrl.UnassignDefect))))
	apiV1.Handle("GET /products/{productId}/defects/{id}/links", authed(http.HandlerFunc(backlogCtrl.ListDefectLinks)))
	apiV1.Handle("POST /products/{productId}/defects/{id}/links", authed(csrf(http.HandlerFunc(backlogCtrl.AddDefectLink))))
	apiV1.Handle("DELETE /products/{productId}/defects/{id}/links/{linkId}", authed(csrf(http.HandlerFunc(backlogCtrl.RemoveDefectLink))))

	// Test cases.
	apiV1.Handle("GET /products/{productId}/test-cases", authed(http.HandlerFunc(testingCtrl.ListTestCases)))
	apiV1.Handle("POST /products/{productId}/test-cases", authed(csrf(http.HandlerFunc(testingCtrl.CreateTestCase))))
	apiV1.Handle("GET /products/{productId}/test-cases/{id}", authed(http.HandlerFunc(testingCtrl.GetTestCase)))
	apiV1.Handle("PATCH /products/{productId}/test-cases/{id}", authed(csrf(http.HandlerFunc(testingCtrl.UpdateTestCase))))
	apiV1.Handle("DELETE /products/{productId}/test-cases/{id}", authed(csrf(http.HandlerFunc(testingCtrl.DeleteTestCase))))
	apiV1.Handle("GET /products/{productId}/test-cases/{id}/requirements", authed(http.HandlerFunc(testingCtrl.ListTestCaseRequirements)))
	apiV1.Handle("POST /products/{productId}/test-cases/{id}/requirements", authed(csrf(http.HandlerFunc(testingCtrl.LinkRequirement))))
	apiV1.Handle("DELETE /products/{productId}/test-cases/{id}/requirements/{reqId}", authed(csrf(http.HandlerFunc(testingCtrl.UnlinkRequirement))))
	apiV1.Handle("GET /products/{productId}/test-cases/{id}/environments", authed(http.HandlerFunc(testingCtrl.ListTestCaseEnvironments)))
	apiV1.Handle("POST /products/{productId}/test-cases/{id}/environments", authed(csrf(http.HandlerFunc(testingCtrl.AddEnvironment))))
	apiV1.Handle("DELETE /products/{productId}/test-cases/{id}/environments/{envId}", authed(csrf(http.HandlerFunc(testingCtrl.RemoveEnvironment))))

	// Campaigns.
	apiV1.Handle("GET /products/{productId}/campaigns", authed(http.HandlerFunc(testingCtrl.ListCampaigns)))
	apiV1.Handle("POST /products/{productId}/campaigns", authed(csrf(http.HandlerFunc(testingCtrl.CreateCampaign))))
	apiV1.Handle("GET /products/{productId}/campaigns/{id}", authed(http.HandlerFunc(testingCtrl.GetCampaign)))
	apiV1.Handle("PATCH /products/{productId}/campaigns/{id}", authed(csrf(http.HandlerFunc(testingCtrl.UpdateCampaign))))
	apiV1.Handle("DELETE /products/{productId}/campaigns/{id}", authed(csrf(http.HandlerFunc(testingCtrl.DeleteCampaign))))
	apiV1.Handle("GET /products/{productId}/campaigns/{id}/results", authed(http.HandlerFunc(testingCtrl.ListCampaignResults)))
	apiV1.Handle("PATCH /products/{productId}/campaigns/{id}/results/{resultId}", authed(csrf(http.HandlerFunc(testingCtrl.UpdateResult))))

	// Comments.
	apiV1.Handle("GET /products/{productId}/comments", authed(http.HandlerFunc(discussionCtrl.ListComments)))
	apiV1.Handle("POST /products/{productId}/comments", authed(csrf(http.HandlerFunc(discussionCtrl.CreateComment))))
	apiV1.Handle("GET /products/{productId}/comments/{id}", authed(http.HandlerFunc(discussionCtrl.GetComment)))
	apiV1.Handle("PATCH /products/{productId}/comments/{id}", authed(csrf(http.HandlerFunc(discussionCtrl.UpdateComment))))
	apiV1.Handle("DELETE /products/{productId}/comments/{id}", authed(csrf(http.HandlerFunc(discussionCtrl.DeleteComment))))

	// Attachments.
	apiV1.Handle("GET /products/{productId}/attachments", authed(http.HandlerFunc(discussionCtrl.ListAttachments)))
	apiV1.Handle("POST /products/{productId}/attachments", authed(csrf(http.HandlerFunc(discussionCtrl.CreateAttachment))))
	apiV1.Handle("GET /products/{productId}/attachments/{id}", authed(http.HandlerFunc(discussionCtrl.GetAttachment)))
	apiV1.Handle("GET /products/{productId}/attachments/{id}/download", authed(http.HandlerFunc(discussionCtrl.DownloadAttachment)))
	apiV1.Handle("DELETE /products/{productId}/attachments/{id}", authed(csrf(http.HandlerFunc(discussionCtrl.DeleteAttachment))))

	// Audit trail.
	apiV1.Handle("GET /products/{productId}/audit", authed(http.HandlerFunc(auditCtrl.List)))

	mux.Handle("/api/v1/", http.StripPrefix("/api/v1", apiV1))
}

// authed wraps a handler with RequireAuth middleware.
func authed(h http.Handler) http.Handler {
	return middleware.RequireAuth(h)
}

// admin wraps a handler with RequireAuth + RequireAdmin middleware.
func admin(h http.Handler) http.Handler {
	return middleware.RequireAdmin(middleware.RequireAuth(h))
}

// csrf wraps a handler with CSRF middleware.
func csrf(h http.Handler) http.Handler {
	return middleware.CSRF(h)
}
