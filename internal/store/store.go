package store

import (
	"database/sql"

	"nexmap.net/internal/crypto"
)

// Store groups all data access interfaces.
type Store struct {
	User               UserStore
	Session            SessionStore
	PAT                PATStore
	Setting            SettingStore
	Invitation         InvitationStore
	Product            ProductStore
	Component          ComponentStore
	Functionality      FunctionalityStore
	Milestone          MilestoneStore
	Status             StatusStore
	Sprint             SprintStore
	Requirement        RequirementStore
	Defect             DefectStore
	PyramidLevel       PyramidLevelStore
	TestCase           TestCaseStore
	RuntimeEnvironment RuntimeEnvironmentStore
	TestingCampaign    TestingCampaignStore
	CampaignResult     CampaignResultStore
	Comment            CommentStore
	Attachment         AttachmentStore
	Audit              AuditStore
}

// New creates a Store backed by the given database connection.
func New(db *sql.DB, keys *crypto.KeyRing) *Store {
	return &Store{
		User:               &userStore{db: db, keys: keys},
		Session:            &sessionStore{db: db},
		PAT:                &patStore{db: db},
		Setting:            &settingStore{db: db},
		Invitation:         &invitationStore{db: db, keys: keys},
		Product:            &productStore{db: db},
		Component:          &componentStore{db: db},
		Functionality:      &functionalityStore{db: db},
		Milestone:          &milestoneStore{db: db},
		Status:             &statusStore{db: db},
		Sprint:             &sprintStore{db: db},
		Requirement:        &requirementStore{db: db},
		Defect:             &defectStore{db: db},
		PyramidLevel:       &pyramidLevelStore{db: db},
		TestCase:           &testCaseStore{db: db},
		RuntimeEnvironment: &runtimeEnvironmentStore{db: db},
		TestingCampaign:    &testingCampaignStore{db: db},
		CampaignResult:     &campaignResultStore{db: db},
		Comment:            &commentStore{db: db},
		Attachment:         &attachmentStore{db: db},
		Audit:              &auditStore{db: db},
	}
}
