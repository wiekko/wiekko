package controller

import (
	"errors"
	"log/slog"
	"net/http"

	"nexmap.net/internal/middleware"
	"nexmap.net/internal/model"
	"nexmap.net/internal/service"
	login "nexmap.net/web/templates/pages/login"
)

// PageRegisterController serves the public account pages: self-registration
// (FR-201, FR-203) and accepting an invitation (FR-202, FR-204, FR-305).
type PageRegisterController struct {
	users *service.UserService
	login *PageLoginController
}

// NewPageRegister creates a PageRegisterController. It signs new accounts in
// through the login controller, so both share one session-cookie path.
func NewPageRegister(users *service.UserService, login *PageLoginController) *PageRegisterController {
	return &PageRegisterController{users: users, login: login}
}

const pendingApprovalMessage = "Your account has been created. An administrator has to approve it before you can sign in."

// ShowRegister handles GET /register.
func (c *PageRegisterController) ShowRegister(w http.ResponseWriter, r *http.Request) {
	if middleware.UserFromContext(r.Context()) != nil {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	if !c.selfRegistrationEnabled(r) {
		c.renderMessage(w, r, http.StatusNotFound, "Registration is closed",
			"This installation does not allow registering an account yourself. Ask an administrator or a product owner for an invitation.")
		return
	}
	c.renderForm(w, r, http.StatusOK, "Create your account", "", login.RegisterFormProps{Action: "/register"})
}

// Register handles POST /register.
func (c *PageRegisterController) Register(w http.ResponseWriter, r *http.Request) {
	if !c.selfRegistrationEnabled(r) {
		c.renderMessage(w, r, http.StatusNotFound, "Registration is closed",
			"This installation does not allow registering an account yourself.")
		return
	}
	props := formValues(r, "/register")
	input, msg := registerInput(r)
	if msg != "" {
		props.Error = msg
		c.renderForm(w, r, http.StatusUnprocessableEntity, "Create your account", "", props)
		return
	}
	user, err := c.users.Register(r.Context(), input)
	if err != nil {
		props.Error = publicAccountError(r, err)
		c.renderForm(w, r, http.StatusUnprocessableEntity, "Create your account", "", props)
		return
	}
	c.signIn(w, r, user, input.Password)
}

// ShowAcceptInvite handles GET /accept-invite/{token}.
func (c *PageRegisterController) ShowAcceptInvite(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	details, err := c.users.LookupInvitation(r.Context(), token)
	if err != nil {
		c.renderInvitationError(w, r, err)
		return
	}
	props := login.RegisterFormProps{
		Action:      "/accept-invite/" + token,
		Email:       details.Email,
		EmailLocked: details.Email != "",
	}
	c.renderForm(w, r, http.StatusOK, "Accept your invitation", invitationIntro(details), props)
}

// AcceptInvite handles POST /accept-invite/{token}.
func (c *PageRegisterController) AcceptInvite(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("token")
	details, err := c.users.LookupInvitation(r.Context(), token)
	if err != nil {
		c.renderInvitationError(w, r, err)
		return
	}
	props := formValues(r, "/accept-invite/"+token)
	if details.Email != "" {
		// The field is disabled in the form, so browsers do not send it.
		props.Email, props.EmailLocked = details.Email, true
	}
	intro := invitationIntro(details)

	input, msg := registerInput(r)
	if msg != "" {
		props.Error = msg
		c.renderForm(w, r, http.StatusUnprocessableEntity, "Accept your invitation", intro, props)
		return
	}
	input.Email = props.Email
	user, err := c.users.AcceptInvitation(r.Context(), token, input)
	if err != nil {
		if isInvitationError(err) {
			c.renderInvitationError(w, r, err)
			return
		}
		props.Error = publicAccountError(r, err)
		c.renderForm(w, r, http.StatusUnprocessableEntity, "Accept your invitation", intro, props)
		return
	}
	c.signIn(w, r, user, input.Password)
}

// signIn starts a session for a freshly created account and sends the browser
// to the dashboard. An account awaiting approval cannot sign in yet, so the
// person is told so instead. Should signing in fail for another reason (e.g.
// the instance enforces 2FA with a second step), the login page takes over.
func (c *PageRegisterController) signIn(w http.ResponseWriter, r *http.Request, user *model.User, password string) {
	if user.Status != model.UserStatusActive {
		if r.Header.Get("HX-Request") == "true" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_ = login.RegisterDone(pendingApprovalMessage).Render(r.Context(), w)
			return
		}
		c.renderMessage(w, r, http.StatusOK, "Account created", pendingApprovalMessage)
		return
	}
	result, err := c.login.auth.Login(r.Context(), user.Username, password, clientIP(r), r.UserAgent())
	if err != nil || result.TOTPRequired {
		redirect(w, r, "/login")
		return
	}
	c.login.finishLogin(w, r, result.SessionID)
}

func (c *PageRegisterController) selfRegistrationEnabled(r *http.Request) bool {
	v, err := c.login.settings.Get(r.Context(), "self_registration_enabled")
	return err == nil && v == "true"
}

func (c *PageRegisterController) renderForm(w http.ResponseWriter, r *http.Request, status int, title, intro string, props login.RegisterFormProps) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if r.Header.Get("HX-Request") == "true" {
		// htmx swaps the response into the form; a 4xx would be discarded.
		_ = login.RegisterForm(props).Render(r.Context(), w)
		return
	}
	w.WriteHeader(status)
	_ = login.RegisterPage(title, intro, props).Render(r.Context(), w)
}

func (c *PageRegisterController) renderMessage(w http.ResponseWriter, r *http.Request, status int, title, message string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_ = login.AccountMessagePage(title, message).Render(r.Context(), w)
}

func (c *PageRegisterController) renderInvitationError(w http.ResponseWriter, r *http.Request, err error) {
	var message string
	switch {
	case errors.Is(err, service.ErrInvitationAccepted):
		message = "This invitation has already been used. If it was yours, sign in with the account you created."
	case errors.Is(err, service.ErrInvitationExpired):
		message = "This invitation has expired. Ask the person who invited you for a new one."
	default:
		message = "This invitation link is not valid. Check that you copied it completely, or ask for a new one."
	}
	if r.Header.Get("HX-Request") == "true" {
		// Replace the whole page: the form is of no use any more.
		w.Header().Set("HX-Retarget", "body")
	}
	c.renderMessage(w, r, http.StatusNotFound, "Invitation not usable", message)
}

func isInvitationError(err error) bool {
	return errors.Is(err, service.ErrInvitationInvalid) ||
		errors.Is(err, service.ErrInvitationAccepted) ||
		errors.Is(err, service.ErrInvitationExpired)
}

func invitationIntro(d *service.InvitationDetails) string {
	if d.ProductName == "" {
		return "You have been invited to Nexmap. Choose a username and password to create your account."
	}
	return "You have been invited to join " + d.ProductName + " as " + d.Role + ". Choose a username and password to create your account."
}

// formValues echoes the submitted non-secret fields back into the form.
func formValues(r *http.Request, action string) login.RegisterFormProps {
	return login.RegisterFormProps{
		Action:      action,
		Username:    r.FormValue("username"),
		Email:       r.FormValue("email"),
		DisplayName: r.FormValue("display_name"),
	}
}

// registerInput reads the form and checks what the service cannot: that the
// password was typed the same way twice.
func registerInput(r *http.Request) (service.RegisterInput, string) {
	input := service.RegisterInput{
		Username:    r.FormValue("username"),
		Email:       r.FormValue("email"),
		DisplayName: r.FormValue("display_name"),
		Password:    r.FormValue("password"),
		ClientIP:    clientIP(r),
	}
	if input.Password != r.FormValue("password_confirm") {
		return input, "The passwords do not match."
	}
	return input, ""
}

// publicAccountError turns a registration error into a message for the form.
// Validation messages from the service are meant for the user; anything else
// is logged and replaced by a generic message, so no internals leak.
func publicAccountError(r *http.Request, err error) string {
	var inputErr *service.AccountInputError
	if wait, blocked := attemptsBlocked(err); blocked {
		return tooManyAccountsMessage(wait)
	}
	switch {
	case errors.Is(err, service.ErrUsernameTaken), errors.Is(err, service.ErrEmailTaken):
		return err.Error()
	case errors.As(err, &inputErr):
		return inputErr.Error()
	}
	slog.ErrorContext(r.Context(), "account creation failed", "err", err)
	return "Your account could not be created. Please try again later."
}

// redirect sends the browser to url, through htmx when the request came from it.
func redirect(w http.ResponseWriter, r *http.Request, url string) {
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Redirect", url)
		w.WriteHeader(http.StatusOK)
		return
	}
	http.Redirect(w, r, url, http.StatusSeeOther)
}
