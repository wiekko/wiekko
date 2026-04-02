package service

import (
	"nexmap.net/internal/auth"
	"nexmap.net/internal/config"
	"nexmap.net/internal/store"
)

// Services groups all service interfaces.
type Services struct {
	Auth       *AuthService
	User       *UserService
	PAT        *PATService
	Setting    *SettingService
	Invitation *InvitationService
}

// New creates the application service layer.
func New(st *store.Store, provider auth.Provider, cfg *config.Config) *Services {
	settingSvc := &SettingService{settings: st.Setting}
	return &Services{
		Auth: &AuthService{
			users:    st.User,
			sessions: st.Session,
			pats:     st.PAT,
			provider: provider,
			cfg:      cfg,
		},
		User: &UserService{
			users:       st.User,
			sessions:    st.Session,
			settings:    st.Setting,
			invitations: st.Invitation,
			provider:    provider,
		},
		PAT: &PATService{
			pats: st.PAT,
		},
		Setting: settingSvc,
		Invitation: &InvitationService{
			invitations: st.Invitation,
		},
	}
}
