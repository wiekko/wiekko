package router

import (
	"net/http"

	"nexmap.net/internal/controller"
	"nexmap.net/internal/middleware"
	"nexmap.net/internal/service"
)

func mountPages(mux *http.ServeMux, svc *service.Services, cookieSecure bool) {
	loginCtrl := controller.NewPageLogin(svc.Auth, svc.Setting, cookieSecure)
	dashCtrl := controller.NewPageDashboard(svc.Dashboard, svc.Setting)
	adminCtrl := controller.NewPageAdmin(svc.User, svc.Setting, svc.Invitation)
	userCtrl := controller.NewPageUser(svc.User, svc.PAT)
	productCtrl := controller.NewPageProduct(svc.Product, svc.Dashboard, svc.Setting, svc.User, svc.Invitation, svc.Milestone, svc.Audit)

	// Public pages.
	mux.HandleFunc("GET /login", loginCtrl.Show)
	mux.HandleFunc("POST /login", loginCtrl.Login)

	// Authenticated pages.
	mux.Handle("GET /{$}", pageAuthed(http.HandlerFunc(dashCtrl.Show)))
	mux.Handle("POST /logout", pageAuthed(http.HandlerFunc(loginCtrl.Logout)))

	// User pages.
	mux.Handle("GET /user/profile", pageAuthed(http.HandlerFunc(userCtrl.Profile)))
	mux.Handle("POST /user/profile", pageAuthed(http.HandlerFunc(userCtrl.UpdateProfile)))
	mux.Handle("POST /user/profile/password", pageAuthed(http.HandlerFunc(userCtrl.ChangePassword)))
	mux.Handle("POST /user/profile/delete", pageAuthed(http.HandlerFunc(userCtrl.RequestDeletion)))
	mux.Handle("GET /user/tokens", pageAuthed(http.HandlerFunc(userCtrl.Tokens)))
	mux.Handle("POST /user/tokens", pageAuthed(http.HandlerFunc(userCtrl.CreateToken)))
	mux.Handle("DELETE /user/tokens/{id}", pageAuthed(http.HandlerFunc(userCtrl.RevokeToken)))

	// Product pages.
	mux.Handle("GET /products/new", pageAuthed(http.HandlerFunc(productCtrl.CreateForm)))
	mux.Handle("POST /products/new", pageAuthed(http.HandlerFunc(productCtrl.Create)))
	mux.Handle("GET /products/{id}", pageAuthed(http.HandlerFunc(productCtrl.Overview)))
	mux.Handle("GET /products/{id}/settings", pageAuthed(http.HandlerFunc(productCtrl.Settings)))
	mux.Handle("GET /products/{id}/settings/general", pageAuthed(http.HandlerFunc(productCtrl.SettingsGeneral)))
	mux.Handle("GET /products/{id}/settings/statuses", pageAuthed(http.HandlerFunc(productCtrl.SettingsStatuses)))
	mux.Handle("GET /products/{id}/settings/environments", pageAuthed(http.HandlerFunc(productCtrl.SettingsEnvironments)))
	mux.Handle("GET /products/{id}/settings/pyramid", pageAuthed(http.HandlerFunc(productCtrl.SettingsPyramid)))
	mux.Handle("PATCH /products/{id}/settings", pageAuthed(http.HandlerFunc(productCtrl.UpdateProduct)))
	mux.Handle("POST /products/{id}/settings/statuses", pageAuthed(http.HandlerFunc(productCtrl.CreateStatus)))
	mux.Handle("PATCH /products/{id}/settings/statuses/{statusId}", pageAuthed(http.HandlerFunc(productCtrl.UpdateStatus)))
	mux.Handle("DELETE /products/{id}/settings/statuses/{statusId}", pageAuthed(http.HandlerFunc(productCtrl.DeleteStatus)))
	mux.Handle("POST /products/{id}/settings/statuses/split", pageAuthed(http.HandlerFunc(productCtrl.SplitStatuses)))
	mux.Handle("POST /products/{id}/settings/environments", pageAuthed(http.HandlerFunc(productCtrl.CreateEnvironment)))
	mux.Handle("DELETE /products/{id}/settings/environments/{envId}", pageAuthed(http.HandlerFunc(productCtrl.DeleteEnvironment)))
	mux.Handle("POST /products/{id}/settings/pyramid", pageAuthed(http.HandlerFunc(productCtrl.CreatePyramidLevel)))
	mux.Handle("PATCH /products/{id}/settings/pyramid/{levelId}", pageAuthed(http.HandlerFunc(productCtrl.UpdatePyramidLevel)))
	mux.Handle("DELETE /products/{id}/settings/pyramid/{levelId}", pageAuthed(http.HandlerFunc(productCtrl.DeletePyramidLevel)))
	mux.Handle("GET /products/{id}/team", pageAuthed(http.HandlerFunc(productCtrl.Team)))
	mux.Handle("POST /products/{id}/team/members", pageAuthed(http.HandlerFunc(productCtrl.AddMember)))
	mux.Handle("PATCH /products/{id}/team/members/{userId}", pageAuthed(http.HandlerFunc(productCtrl.UpdateMemberRole)))
	mux.Handle("DELETE /products/{id}/team/members/{userId}", pageAuthed(http.HandlerFunc(productCtrl.RemoveMember)))
	mux.Handle("POST /products/{id}/team/transfer", pageAuthed(http.HandlerFunc(productCtrl.TransferOwnership)))
	mux.Handle("GET /products/{id}/team/search-users", pageAuthed(http.HandlerFunc(productCtrl.SearchUsers)))
	mux.Handle("POST /products/{id}/team/invite", pageAuthed(http.HandlerFunc(productCtrl.InviteMember)))
	mux.Handle("GET /products/{id}/team/invite-form", pageAuthed(http.HandlerFunc(productCtrl.InviteFormReset)))
	mux.Handle("DELETE /products/{id}", pageAuthed(http.HandlerFunc(productCtrl.DeleteProduct)))
	mux.Handle("GET /products/{id}/delete/confirm", pageAuthed(http.HandlerFunc(productCtrl.DeleteProductConfirm)))

	// Milestone pages.
	mux.Handle("GET /products/{id}/milestones", pageAuthed(http.HandlerFunc(productCtrl.Milestones)))
	mux.Handle("GET /products/{id}/milestones/new", pageAuthed(http.HandlerFunc(productCtrl.MilestoneCreateForm)))
	mux.Handle("POST /products/{id}/milestones", pageAuthed(http.HandlerFunc(productCtrl.MilestoneCreate)))
	mux.Handle("GET /products/{id}/milestones/{milestoneId}", pageAuthed(http.HandlerFunc(productCtrl.MilestoneDetail)))
	mux.Handle("GET /products/{id}/milestones/{milestoneId}/edit", pageAuthed(http.HandlerFunc(productCtrl.MilestoneEditForm)))
	mux.Handle("POST /products/{id}/milestones/{milestoneId}", pageAuthed(http.HandlerFunc(productCtrl.MilestoneUpdate)))
	mux.Handle("DELETE /products/{id}/milestones/{milestoneId}", pageAuthed(http.HandlerFunc(productCtrl.MilestoneDelete)))
	mux.Handle("GET /products/{id}/milestones/{milestoneId}/delete/confirm", pageAuthed(http.HandlerFunc(productCtrl.MilestoneDeleteConfirm)))
	mux.Handle("GET /products/{id}/milestones/{milestoneId}/tabs/overview", pageAuthed(http.HandlerFunc(productCtrl.MilestoneOverviewTab)))
	mux.Handle("GET /products/{id}/milestones/{milestoneId}/tabs/items", pageAuthed(http.HandlerFunc(productCtrl.MilestoneItemsTab)))
	mux.Handle("GET /products/{id}/milestones/{milestoneId}/tabs/sprints", pageAuthed(http.HandlerFunc(productCtrl.MilestoneSprintsTab)))
	mux.Handle("GET /products/{id}/milestones/{milestoneId}/tabs/history", pageAuthed(http.HandlerFunc(productCtrl.MilestoneHistoryTab)))

	// Admin pages.
	mux.Handle("GET /admin/users", pageAdmin(http.HandlerFunc(adminCtrl.ListUsers)))
	mux.Handle("GET /admin/users/{id}", pageAdmin(http.HandlerFunc(adminCtrl.GetUser)))
	mux.Handle("POST /admin/users/{id}/approve", pageAdmin(http.HandlerFunc(adminCtrl.ApproveUser)))
	mux.Handle("POST /admin/users/{id}/lock", pageAdmin(http.HandlerFunc(adminCtrl.LockUser)))
	mux.Handle("POST /admin/users/{id}/unlock", pageAdmin(http.HandlerFunc(adminCtrl.UnlockUser)))
	mux.Handle("POST /admin/users/{id}/password", pageAdmin(http.HandlerFunc(adminCtrl.SetPassword)))
	mux.Handle("DELETE /admin/users/{id}", pageAdmin(http.HandlerFunc(adminCtrl.DeleteUser)))
	mux.Handle("GET /admin/settings", pageAdmin(http.HandlerFunc(adminCtrl.Settings)))
	mux.Handle("PATCH /admin/settings", pageAdmin(http.HandlerFunc(adminCtrl.UpdateSetting)))
	mux.Handle("GET /admin/invitations", pageAdmin(http.HandlerFunc(adminCtrl.Invitations)))
	mux.Handle("POST /admin/invitations", pageAdmin(http.HandlerFunc(adminCtrl.CreateInvitation)))
	mux.Handle("DELETE /admin/invitations/{id}", pageAdmin(http.HandlerFunc(adminCtrl.DeleteInvitation)))
}

// pageAuthed wraps a handler with RequireAuthPage middleware (redirect to /login).
func pageAuthed(h http.Handler) http.Handler {
	return middleware.RequireAuthPage(h)
}

// pageAdmin wraps a handler with RequireAdminPage middleware (redirect to / for non-admins).
func pageAdmin(h http.Handler) http.Handler {
	return middleware.RequireAdminPage(h)
}
