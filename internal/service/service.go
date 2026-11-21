package service

import (
	"nexmap.net/internal/auth"
	"nexmap.net/internal/config"
	"nexmap.net/internal/mail"
	"nexmap.net/internal/storage"
	"nexmap.net/internal/store"
)

// Services groups all service interfaces.
type Services struct {
	Auth       *AuthService
	TwoFactor  *TwoFactorService
	User       *UserService
	PAT        *PATService
	Setting    *SettingService
	Invitation *InvitationService
	// PasswordReset is enabled only when Mail is.
	PasswordReset *PasswordResetService
	// Mail delivers invitation and password reset emails; run Mail.Run in the
	// background for the latter.
	Mail       *Mailer
	Product    *ProductService
	Component  *ComponentService
	Milestone  *MilestoneService
	Category   *CategoryService
	Backlog    *BacklogService
	Constraint *ConstraintService
	Document   *DocumentService
	Testing    *TestingService
	TestRun    *TestRunService
	Discussion *DiscussionService
	Audit      *AuditService
	Dashboard  *DashboardService
	Spec       *SpecificationService
}

// New creates the application service layer without mail: invitations are
// links only and there is no password reset.
func New(st *store.Store, provider auth.Provider, cfg *config.Config, blobs storage.Store) *Services {
	return NewWithMail(st, provider, cfg, blobs, nil)
}

// NewWithMail creates the application service layer sending email through
// sender (nil: no mail, as New).
func NewWithMail(st *store.Store, provider auth.Provider, cfg *config.Config, blobs storage.Store, sender mail.Sender) *Services {
	mailer := NewMailer(sender, cfg.PublicURL)
	settingSvc := &SettingService{settings: st.Setting}

	authz := &Authorizer{productRoles: st.ProductRole, products: st.Product}
	owns := newOwnership(st)

	// Built first: the entity services below delegate their comment and
	// attachment cleanup to it when they delete a row.
	discussionSvc := &DiscussionService{
		tx:          st.Tx,
		comments:    st.Comment,
		attachments: st.Attachment,
		defectLinks: st.Defect,
		audit:       st.Audit,
		blobs:       blobs,
		policy:      storage.NewPolicy(cfg.AllowedUploads),
		authz:       authz,
		owns:        owns,
	}

	// Built before the backlog service, which calls into it to reopen campaign
	// results once the defect that failed them is closed (FR-123).
	testingSvc := &TestingService{
		tx:              st.Tx,
		testCases:       st.TestCase,
		constraints:     st.Constraint,
		capabilities:    st.Capability,
		campaigns:       st.TestingCampaign,
		campaignResults: st.CampaignResult,
		campaignEvents:  st.CampaignEvent,
		comments:        discussionSvc,
		audit:           st.Audit,
		authz:           authz,
		owns:            owns,
	}

	twoFactorSvc := &TwoFactorService{
		tx:           st.Tx,
		users:        st.User,
		twoFactor:    st.TwoFactor,
		settings:     st.Setting,
		productRoles: st.ProductRole,
		products:     st.Product,
		provider:     provider,
		limiter:      newAttemptLimiter(secondFactorMaxFailures, SecondFactorFailureWindow),
	}

	// Built before the test run service, which files defects for automated
	// failures through it.
	backlogSvc := &BacklogService{
		tx:           st.Tx,
		requirements: st.Requirement,
		defects:      st.Defect,
		categories:   st.Category,
		statuses:     st.Status,
		capabilities: st.Capability,
		products:     st.Product,
		comments:     discussionSvc,
		retester:     testingSvc,
		audit:        st.Audit,
		authz:        authz,
		owns:         owns,
	}

	userSvc := &UserService{
		tx:           st.Tx,
		users:        st.User,
		sessions:     st.Session,
		settings:     st.Setting,
		invitations:  st.Invitation,
		products:     st.Product,
		productRoles: st.ProductRole,
		provider:     provider,
		regLimiter:   newAttemptLimiter(registrationMaxPerAddress, RegistrationWindow),
	}

	return &Services{
		Mail: mailer,
		Auth: &AuthService{
			tx:             st.Tx,
			users:          st.User,
			sessions:       st.Session,
			pats:           st.PAT,
			provider:       provider,
			cfg:            cfg,
			twoFactor:      twoFactorSvc,
			twoFactorStore: st.TwoFactor,
			limiter:        newLoginLimiter(),
		},
		TwoFactor: twoFactorSvc,
		User:      userSvc,
		PasswordReset: &PasswordResetService{
			tx:          st.Tx,
			users:       st.User,
			resets:      st.PasswordReset,
			accounts:    userSvc,
			mail:        mailer,
			ipLimiter:   newAttemptLimiter(passwordResetsPerAddress, PasswordResetWindow),
			userLimiter: newAttemptLimiter(passwordResetMailsPerUser, PasswordResetWindow),
		},
		PAT: &PATService{
			tx:           st.Tx,
			pats:         st.PAT,
			productRoles: st.ProductRole,
			settings:     st.Setting,
			provider:     provider,
		},
		Setting: settingSvc,
		Invitation: &InvitationService{
			tx:               st.Tx,
			invitations:      st.Invitation,
			authz:            authz,
			publicURL:        cfg.PublicURL,
			mail:             mailer,
			inviterLimiter:   newAttemptLimiter(invitationMailsPerInviter, InvitationMailWindow),
			recipientLimiter: newAttemptLimiter(invitationMailsPerRecipient, InvitationRecipientWindow),
		},
		Product: &ProductService{
			tx:            st.Tx,
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
			tx:           st.Tx,
			components:   st.Component,
			capabilities: st.Capability,
			comments:     discussionSvc,
			audit:        st.Audit,
			authz:        authz,
			owns:         owns,
		},
		Milestone: &MilestoneService{
			tx:            st.Tx,
			milestones:    st.Milestone,
			sprints:       st.Sprint,
			statuses:      st.Status,
			products:      st.Product,
			pyramidLevels: st.PyramidLevel,
			runtimeEnvs:   st.RuntimeEnvironment,
			results:       st.CampaignResult,
			comments:      discussionSvc,
			audit:         st.Audit,
			authz:         authz,
			owns:          owns,
			archive:       backlogSvc.archiver(),
		},
		Category: &CategoryService{
			tx:           st.Tx,
			categories:   st.Category,
			products:     st.Product,
			requirements: st.Requirement,
			constraints:  st.Constraint,
			audit:        st.Audit,
			authz:        authz,
			owns:         owns,
		},
		Backlog: backlogSvc,
		Document: &DocumentService{
			tx:           st.Tx,
			documents:    st.Document,
			products:     st.Product,
			requirements: st.Requirement,
			constraints:  st.Constraint,
			testCases:    st.TestCase,
			categories:   st.Category,
			comments:     discussionSvc,
			audit:        st.Audit,
			authz:        authz,
			owns:         owns,
		},
		Spec: &SpecificationService{
			products:     st.Product,
			components:   st.Component,
			capabilities: st.Capability,
			categories:   st.Category,
			requirements: st.Requirement,
			constraints:  st.Constraint,
			documents:    st.Document,
			testCases:    st.TestCase,
			authz:        authz,
		},
		Constraint: &ConstraintService{
			tx:          st.Tx,
			constraints: st.Constraint,
			categories:  st.Category,
			comments:    discussionSvc,
			audit:       st.Audit,
			authz:       authz,
			owns:        owns,
		},
		Testing: testingSvc,
		TestRun: &TestRunService{
			tx:           st.Tx,
			testCases:    st.TestCase,
			campaigns:    st.TestingCampaign,
			results:      st.CampaignResult,
			events:       st.CampaignEvent,
			environments: st.RuntimeEnvironment,
			defects:      st.Defect,
			statuses:     st.Status,
			backlog:      backlogSvc,
			authz:        authz,
			owns:         owns,
		},
		Discussion: discussionSvc,
		Audit: &AuditService{
			audit:    st.Audit,
			products: st.Product,
			tx:       st.Tx,
			authz:    authz,
			owns:     owns,
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
