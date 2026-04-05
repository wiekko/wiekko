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
	Product    *ProductService
	Component  *ComponentService
	Milestone  *MilestoneService
	Backlog    *BacklogService
	Testing    *TestingService
	Comment    *CommentService
	Audit      *AuditService
	Dashboard  *DashboardService
}

// New creates the application service layer.
func New(st *store.Store, provider auth.Provider, cfg *config.Config) *Services {
	settingSvc := &SettingService{settings: st.Setting}

	authz := &Authorizer{productRoles: st.ProductRole}

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
		Product: &ProductService{
			products:      st.Product,
			productRoles:  st.ProductRole,
			statuses:      st.Status,
			pyramidLevels: st.PyramidLevel,
			settings:      st.Setting,
			audit:         st.Audit,
			authz:         authz,
		},
		Component: &ComponentService{
			components:      st.Component,
			functionalities: st.Functionality,
			audit:           st.Audit,
			authz:           authz,
		},
		Milestone: &MilestoneService{
			milestones:    st.Milestone,
			sprints:       st.Sprint,
			statuses:      st.Status,
			products:      st.Product,
			pyramidLevels: st.PyramidLevel,
			runtimeEnvs:   st.RuntimeEnvironment,
			audit:         st.Audit,
			authz:         authz,
		},
		Backlog: &BacklogService{
			requirements: st.Requirement,
			defects:      st.Defect,
			audit:        st.Audit,
			authz:        authz,
		},
		Testing: &TestingService{
			testCases:       st.TestCase,
			campaigns:       st.TestingCampaign,
			campaignResults: st.CampaignResult,
			audit:           st.Audit,
			authz:           authz,
		},
		Comment: &CommentService{
			comments:    st.Comment,
			attachments: st.Attachment,
			audit:       st.Audit,
			authz:       authz,
		},
		Audit: &AuditService{
			audit: st.Audit,
			authz: authz,
		},
		Dashboard: &DashboardService{
			products:     st.Product,
			productRoles: st.ProductRole,
			requirements: st.Requirement,
			defects:      st.Defect,
			statuses:     st.Status,
			sprints:      st.Sprint,
		},
	}
}
