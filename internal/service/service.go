package service

import (
	"nexmap.net/internal/auth"
	"nexmap.net/internal/config"
	"nexmap.net/internal/storage"
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
	Category   *CategoryService
	Backlog    *BacklogService
	Constraint *ConstraintService
	Testing    *TestingService
	Discussion *DiscussionService
	Audit      *AuditService
	Dashboard  *DashboardService
}

// New creates the application service layer.
func New(st *store.Store, provider auth.Provider, cfg *config.Config, blobs storage.Store) *Services {
	settingSvc := &SettingService{settings: st.Setting}

	authz := &Authorizer{productRoles: st.ProductRole}

	// Built first: the entity services below delegate their comment and
	// attachment cleanup to it when they delete a row.
	discussionSvc := &DiscussionService{
		comments:    st.Comment,
		attachments: st.Attachment,
		audit:       st.Audit,
		blobs:       blobs,
		policy:      storage.NewPolicy(cfg.AllowedUploads),
		authz:       authz,
	}

	// Built before the backlog service, which calls into it to reopen campaign
	// results once the defect that failed them is closed (FR-504).
	testingSvc := &TestingService{
		testCases:       st.TestCase,
		campaigns:       st.TestingCampaign,
		campaignResults: st.CampaignResult,
		campaignEvents:  st.CampaignEvent,
		comments:        discussionSvc,
		audit:           st.Audit,
		authz:           authz,
	}

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
			categories:    st.Category,
			pyramidLevels: st.PyramidLevel,
			settings:      st.Setting,
			attachments:   discussionSvc,
			audit:         st.Audit,
			authz:         authz,
		},
		Component: &ComponentService{
			components:   st.Component,
			capabilities: st.Capability,
			comments:     discussionSvc,
			audit:        st.Audit,
			authz:        authz,
		},
		Milestone: &MilestoneService{
			milestones:    st.Milestone,
			sprints:       st.Sprint,
			statuses:      st.Status,
			products:      st.Product,
			pyramidLevels: st.PyramidLevel,
			runtimeEnvs:   st.RuntimeEnvironment,
			comments:      discussionSvc,
			audit:         st.Audit,
			authz:         authz,
		},
		Category: &CategoryService{
			categories:   st.Category,
			requirements: st.Requirement,
			constraints:  st.Constraint,
			audit:        st.Audit,
			authz:        authz,
		},
		Backlog: &BacklogService{
			requirements: st.Requirement,
			defects:      st.Defect,
			categories:   st.Category,
			statuses:     st.Status,
			comments:     discussionSvc,
			retester:     testingSvc,
			audit:        st.Audit,
			authz:        authz,
		},
		Constraint: &ConstraintService{
			constraints: st.Constraint,
			categories:  st.Category,
			comments:    discussionSvc,
			audit:       st.Audit,
			authz:       authz,
		},
		Testing:    testingSvc,
		Discussion: discussionSvc,
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
