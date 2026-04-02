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
