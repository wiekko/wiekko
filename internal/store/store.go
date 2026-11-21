// SPDX-FileCopyrightText: 2026 Jesco Freund (daemotron)
// SPDX-License-Identifier: EUPL-1.2

package store

import (
	"database/sql"

	"nexmap.net/internal/crypto"
)

// Store groups all data access interfaces.
type Store struct {
	// Tx runs several store calls as one transaction.
	Tx Transactor

	User               UserStore
	Session            SessionStore
	TwoFactor          TwoFactorStore
	PAT                PATStore
	Setting            SettingStore
	Invitation         InvitationStore
	PasswordReset      PasswordResetStore
	ProductRole        ProductRoleStore
	Product            ProductStore
	Component          ComponentStore
	Capability         CapabilityStore
	Milestone          MilestoneStore
	Status             StatusStore
	Category           CategoryStore
	Sprint             SprintStore
	Requirement        RequirementStore
	Defect             DefectStore
	Constraint         ConstraintStore
	Document           DocumentStore
	PyramidLevel       PyramidLevelStore
	TestCase           TestCaseStore
	RuntimeEnvironment RuntimeEnvironmentStore
	TestingCampaign    TestingCampaignStore
	CampaignResult     CampaignResultStore
	CampaignEvent      CampaignResultEventStore
	Comment            CommentStore
	Attachment         AttachmentStore
	Audit              AuditStore
}

// New creates a Store backed by the given database connection. dialect selects
// how the portable queries are adapted to the engine (see DB).
func New(conn *sql.DB, dialect Dialect, keys *crypto.KeyRing) *Store {
	db := NewDB(conn, dialect)
	return &Store{
		Tx:                 db,
		User:               &userStore{db: db, keys: keys},
		Session:            &sessionStore{db: db},
		TwoFactor:          &twoFactorStore{db: db},
		PAT:                &patStore{db: db},
		Setting:            &settingStore{db: db},
		Invitation:         &invitationStore{db: db, keys: keys},
		PasswordReset:      &passwordResetStore{db: db},
		ProductRole:        &productRoleStore{db: db},
		Product:            &productStore{db: db},
		Component:          &componentStore{db: db},
		Capability:         &capabilityStore{db: db},
		Milestone:          &milestoneStore{db: db},
		Status:             &statusStore{db: db},
		Category:           &categoryStore{db: db},
		Sprint:             &sprintStore{db: db},
		Requirement:        &requirementStore{db: db},
		Defect:             &defectStore{db: db},
		Constraint:         &constraintStore{db: db},
		Document:           &documentStore{db: db},
		PyramidLevel:       &pyramidLevelStore{db: db},
		TestCase:           &testCaseStore{db: db},
		RuntimeEnvironment: &runtimeEnvironmentStore{db: db},
		TestingCampaign:    &testingCampaignStore{db: db},
		CampaignResult:     &campaignResultStore{db: db},
		CampaignEvent:      &campaignResultEventStore{db: db},
		Comment:            &commentStore{db: db},
		Attachment:         &attachmentStore{db: db},
		Audit:              &auditStore{db: db},
	}
}
