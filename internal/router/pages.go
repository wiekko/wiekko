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
