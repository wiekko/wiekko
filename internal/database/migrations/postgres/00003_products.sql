-- Product: top-level entity (FR-001)
CREATE TABLE IF NOT EXISTS products (
    id              UUID PRIMARY KEY,
    name            TEXT NOT NULL UNIQUE,
    description     TEXT NOT NULL DEFAULT '',
    estimation_mode TEXT NOT NULL DEFAULT 'disabled',
    workflow_mode   TEXT NOT NULL DEFAULT 'kanban',
    status_sync    BOOLEAN NOT NULL DEFAULT TRUE,
    -- Prefix of defect identifiers such as DF-17 (FR-043). Unique within the
    -- product together with the category prefixes, see categories.
    defect_prefix   TEXT NOT NULL DEFAULT 'DF',
    -- Prefix of document identifiers such as DOC-3 (FR-050). Unique within
    -- the product together with the defect and category prefixes.
    document_prefix TEXT NOT NULL DEFAULT 'DOC',
    -- Members must have 2FA set up to reach the product (SC-107).
    two_factor_required BOOLEAN NOT NULL DEFAULT FALSE,
    created_at      TIMESTAMPTZ NOT NULL,
    updated_at      TIMESTAMPTZ NOT NULL
);

-- Product roles: relocated from 00002 with proper FK (SC-201..SC-206)
CREATE TABLE IF NOT EXISTS product_roles (
    id         UUID PRIMARY KEY,
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    product_id UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    role       TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    UNIQUE(user_id, product_id)
);
CREATE INDEX IF NOT EXISTS idx_product_roles_user_id ON product_roles(user_id);
CREATE INDEX IF NOT EXISTS idx_product_roles_product_id ON product_roles(product_id);

-- Products a restricted personal access token reaches (SC-402). Deleting a
-- product removes it here while the token stays restricted, so the token
-- reaches less rather than more.
CREATE TABLE IF NOT EXISTS personal_access_token_products (
    token_id   UUID NOT NULL REFERENCES personal_access_tokens(id) ON DELETE CASCADE,
    product_id UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    PRIMARY KEY (token_id, product_id)
);
CREATE INDEX IF NOT EXISTS idx_pat_products_product_id ON personal_access_token_products(product_id);

-- Milestones: product roadmap (FR-011, FR-012)
CREATE TABLE IF NOT EXISTS milestones (
    id          UUID PRIMARY KEY,
    product_id  UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    type        TEXT NOT NULL DEFAULT 'planning',
    due_date    TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL,
    updated_at  TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_milestones_product_id ON milestones(product_id);

-- Components: product decomposition (FR-002)
CREATE TABLE IF NOT EXISTS components (
    id           UUID PRIMARY KEY,
    product_id   UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    name         TEXT NOT NULL,
    description  TEXT NOT NULL DEFAULT '',
    milestone_id UUID REFERENCES milestones(id) ON DELETE SET NULL,
    created_at   TIMESTAMPTZ NOT NULL,
    updated_at   TIMESTAMPTZ NOT NULL,
    UNIQUE(product_id, name)
);
CREATE INDEX IF NOT EXISTS idx_components_product_id ON components(product_id);
CREATE INDEX IF NOT EXISTS idx_components_milestone_id ON components(milestone_id);

-- Capabilities: linked to components or top-level (FR-003, FR-004)
CREATE TABLE IF NOT EXISTS capabilities (
    id           UUID PRIMARY KEY,
    product_id   UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    component_id UUID REFERENCES components(id) ON DELETE SET NULL,
    name         TEXT NOT NULL,
    description  TEXT NOT NULL DEFAULT '',
    milestone_id UUID REFERENCES milestones(id) ON DELETE SET NULL,
    created_at   TIMESTAMPTZ NOT NULL,
    updated_at   TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_capabilities_product_id ON capabilities(product_id);
CREATE INDEX IF NOT EXISTS idx_capabilities_component_id ON capabilities(component_id);
CREATE INDEX IF NOT EXISTS idx_capabilities_milestone_id ON capabilities(milestone_id);

-- Configurable statuses for requirements and defects (FR-323)
CREATE TABLE IF NOT EXISTS statuses (
    id          UUID PRIMARY KEY,
    product_id  UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    entity_type TEXT NOT NULL,
    name        TEXT NOT NULL,
    position    INTEGER NOT NULL DEFAULT 0,
    is_default  BOOLEAN NOT NULL DEFAULT FALSE,
    is_closed   BOOLEAN NOT NULL DEFAULT FALSE,
    -- Kanban: items in an active column are activated for work (FR-318, FR-323a)
    is_active   BOOLEAN NOT NULL DEFAULT FALSE,
    created_at  TIMESTAMPTZ NOT NULL,
    UNIQUE(product_id, entity_type, name)
);
CREATE INDEX IF NOT EXISTS idx_statuses_product_id ON statuses(product_id, entity_type);

-- Configurable categories for requirements and constraints
CREATE TABLE IF NOT EXISTS categories (
    id          UUID PRIMARY KEY,
    product_id  UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    entity_type TEXT NOT NULL,
    name        TEXT NOT NULL,
    prefix      TEXT NOT NULL,
    position    INTEGER NOT NULL DEFAULT 0,
    created_at  TIMESTAMPTZ NOT NULL,
    UNIQUE(product_id, entity_type, name),
    -- Prefixes are product-wide unique, not per entity type: requirements and
    -- tasks share a sequence counter but constraints have their own, so two
    -- categories of different types sharing a prefix would render two different
    -- items as the same identifier (e.g. FR-1).
    UNIQUE(product_id, prefix)
);
CREATE INDEX IF NOT EXISTS idx_categories_product_id ON categories(product_id, entity_type);

-- Sprints: time-boxed iterations (FR-028, FR-029)
CREATE TABLE IF NOT EXISTS sprints (
    id           UUID PRIMARY KEY,
    product_id   UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    name         TEXT NOT NULL,
    milestone_id UUID REFERENCES milestones(id) ON DELETE SET NULL,
    status       TEXT NOT NULL DEFAULT 'planned',
    start_date   TIMESTAMPTZ NOT NULL,
    end_date     TIMESTAMPTZ NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL,
    updated_at   TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_sprints_product_id ON sprints(product_id);
CREATE INDEX IF NOT EXISTS idx_sprints_milestone_id ON sprints(milestone_id);
-- At most one active sprint per product (FR-318), like campaigns below.
CREATE UNIQUE INDEX IF NOT EXISTS idx_sprints_active ON sprints(product_id) WHERE status = 'active';

-- Requirements: backlog items (FR-005..FR-010, FR-013)
CREATE TABLE IF NOT EXISTS requirements (
    id                 UUID PRIMARY KEY,
    product_id         UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    kind               TEXT NOT NULL DEFAULT 'requirement',
    parent_id          UUID REFERENCES requirements(id) ON DELETE SET NULL,
    category_id        UUID NOT NULL REFERENCES categories(id) ON DELETE RESTRICT,
    title              TEXT NOT NULL,
    description        TEXT NOT NULL DEFAULT '',
    definition_of_done TEXT NOT NULL DEFAULT '',
    status_id          UUID REFERENCES statuses(id) ON DELETE SET NULL,
    milestone_id       UUID REFERENCES milestones(id) ON DELETE SET NULL,
    sprint_id          UUID REFERENCES sprints(id) ON DELETE SET NULL,
    estimation         TEXT,
    priority           INTEGER NOT NULL DEFAULT 0,
    sequence_number    INTEGER NOT NULL DEFAULT 0,
    is_coarse          BOOLEAN NOT NULL DEFAULT FALSE,
    is_active          BOOLEAN NOT NULL DEFAULT FALSE,
    created_at         TIMESTAMPTZ NOT NULL,
    updated_at         TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_requirements_product_id ON requirements(product_id);
CREATE INDEX IF NOT EXISTS idx_requirements_kind ON requirements(product_id, kind);
CREATE INDEX IF NOT EXISTS idx_requirements_parent_id ON requirements(parent_id);
CREATE INDEX IF NOT EXISTS idx_requirements_category_id ON requirements(category_id);
CREATE INDEX IF NOT EXISTS idx_requirements_status_id ON requirements(status_id);
CREATE INDEX IF NOT EXISTS idx_requirements_milestone_id ON requirements(milestone_id);
CREATE INDEX IF NOT EXISTS idx_requirements_sprint_id ON requirements(sprint_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_requirements_product_seq ON requirements(product_id, sequence_number);

-- Defects: bugs and issues (FR-022..FR-025)
-- Runtime environments: freely definable labels (FR-019, FR-020).
-- Defined before defects, which reference an environment.
CREATE TABLE IF NOT EXISTS runtime_environments (
    id         UUID PRIMARY KEY,
    product_id UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    UNIQUE(product_id, name)
);
CREATE INDEX IF NOT EXISTS idx_runtime_environments_product_id ON runtime_environments(product_id);

-- environment_id records where the defect was observed ("login fails in Firefox").
-- It clears rather than cascades: a defect outlives the
-- environment label it was found in.
CREATE TABLE IF NOT EXISTS defects (
    id             UUID PRIMARY KEY,
    product_id     UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    title          TEXT NOT NULL,
    description    TEXT NOT NULL DEFAULT '',
    criticality    TEXT NOT NULL DEFAULT 'medium',
    status_id      UUID REFERENCES statuses(id) ON DELETE SET NULL,
    milestone_id   UUID REFERENCES milestones(id) ON DELETE SET NULL,
    sprint_id      UUID REFERENCES sprints(id) ON DELETE SET NULL,
    environment_id UUID REFERENCES runtime_environments(id) ON DELETE SET NULL,
    estimation     TEXT,
    priority       INTEGER NOT NULL DEFAULT 0,
    is_active      BOOLEAN NOT NULL DEFAULT FALSE,
    -- Defects number from a per-product counter of their own (FR-043).
    sequence_number INTEGER NOT NULL DEFAULT 0,
    created_at     TIMESTAMPTZ NOT NULL,
    updated_at     TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_defects_product_id ON defects(product_id);
CREATE INDEX IF NOT EXISTS idx_defects_status_id ON defects(status_id);
CREATE INDEX IF NOT EXISTS idx_defects_milestone_id ON defects(milestone_id);
CREATE INDEX IF NOT EXISTS idx_defects_sprint_id ON defects(sprint_id);
CREATE INDEX IF NOT EXISTS idx_defects_environment_id ON defects(environment_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_defects_product_seq ON defects(product_id, sequence_number);

-- Constraints: non-functional requirements (architecture, security, UX rules)
CREATE TABLE IF NOT EXISTS constraints (
    id               UUID PRIMARY KEY,
    product_id       UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    category_id      UUID NOT NULL REFERENCES categories(id) ON DELETE RESTRICT,
    sequence_number  INTEGER NOT NULL DEFAULT 0,
    title            TEXT NOT NULL,
    description      TEXT NOT NULL DEFAULT '',
    compliance_state TEXT NOT NULL DEFAULT 'not_assessed',
    milestone_id     UUID REFERENCES milestones(id) ON DELETE SET NULL,
    priority         INTEGER NOT NULL DEFAULT 0,
    created_at       TIMESTAMPTZ NOT NULL,
    updated_at       TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_constraints_product_id ON constraints(product_id);
CREATE INDEX IF NOT EXISTS idx_constraints_category_id ON constraints(category_id);
CREATE INDEX IF NOT EXISTS idx_constraints_milestone_id ON constraints(milestone_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_constraints_product_seq ON constraints(product_id, sequence_number);

-- Pyramid levels: test categorization hierarchy (FR-032..FR-035)
CREATE TABLE IF NOT EXISTS pyramid_levels (
    id         UUID PRIMARY KEY,
    product_id UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    parent_id  UUID REFERENCES pyramid_levels(id) ON DELETE SET NULL,
    name       TEXT NOT NULL,
    position   INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL,
    UNIQUE(product_id, name)
);
CREATE INDEX IF NOT EXISTS idx_pyramid_levels_product_id ON pyramid_levels(product_id);

-- Test cases (FR-014..FR-018, FR-032)
CREATE TABLE IF NOT EXISTS test_cases (
    id               UUID PRIMARY KEY,
    product_id       UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    component_id     UUID REFERENCES components(id) ON DELETE SET NULL,
    capability_id    UUID REFERENCES capabilities(id) ON DELETE SET NULL,
    title            TEXT NOT NULL,
    description      TEXT NOT NULL DEFAULT '',
    execution_mode   TEXT NOT NULL DEFAULT 'manual',
    test_identifier  TEXT,
    pyramid_level_id UUID REFERENCES pyramid_levels(id) ON DELETE SET NULL,
    created_at       TIMESTAMPTZ NOT NULL,
    updated_at       TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_test_cases_product_id ON test_cases(product_id);
CREATE INDEX IF NOT EXISTS idx_test_cases_pyramid_level_id ON test_cases(pyramid_level_id);

-- Test case environment matrix (FR-019)
CREATE TABLE IF NOT EXISTS test_case_environments (
    id             UUID PRIMARY KEY,
    test_case_id   UUID NOT NULL REFERENCES test_cases(id) ON DELETE CASCADE,
    environment_id UUID NOT NULL REFERENCES runtime_environments(id) ON DELETE CASCADE,
    UNIQUE(test_case_id, environment_id)
);
CREATE INDEX IF NOT EXISTS idx_test_case_environments_test_case_id ON test_case_environments(test_case_id);

-- Testing campaigns (FR-320, FR-322, FR-332)
CREATE TABLE IF NOT EXISTS testing_campaigns (
    id          UUID PRIMARY KEY,
    product_id  UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    status      TEXT NOT NULL DEFAULT 'draft',
    starts_at   TIMESTAMPTZ,
    ends_at     TIMESTAMPTZ,
    closed_at   TIMESTAMPTZ,
    closed_by   UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at  TIMESTAMPTZ NOT NULL,
    updated_at  TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_testing_campaigns_product_id ON testing_campaigns(product_id);
-- At most one active campaign per product (FR-322)
CREATE UNIQUE INDEX IF NOT EXISTS idx_testing_campaigns_active ON testing_campaigns(product_id) WHERE status = 'active';

-- Campaign tests: a campaign's own copy of each test case in its scope. The
-- test case is the template; once in a campaign it becomes that campaign's
-- test. Draft campaigns follow edits to the test case, from activation on the
-- copy is frozen, and it outlives the test case: deleting one detaches the copy
-- (test_case_id becomes NULL) so closed campaigns keep their history.
CREATE TABLE IF NOT EXISTS campaign_tests (
    id              UUID PRIMARY KEY,
    campaign_id     UUID NOT NULL REFERENCES testing_campaigns(id) ON DELETE CASCADE,
    test_case_id    UUID REFERENCES test_cases(id) ON DELETE SET NULL,
    title           TEXT NOT NULL,
    description     TEXT NOT NULL DEFAULT '',
    execution_mode  TEXT NOT NULL DEFAULT 'manual',
    test_identifier TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL,
    UNIQUE(campaign_id, test_case_id)
);
CREATE INDEX IF NOT EXISTS idx_campaign_tests_test_case_id ON campaign_tests(test_case_id);

-- Campaign results: status per campaign test x environment (FR-021)
-- environment_name keeps the label: deleting an environment detaches the row
-- (environment_id becomes NULL) but the result still says where it ran. A NULL
-- name marks a test without an environment matrix.
CREATE TABLE IF NOT EXISTS campaign_results (
    id               UUID PRIMARY KEY,
    campaign_id      UUID NOT NULL REFERENCES testing_campaigns(id) ON DELETE CASCADE,
    campaign_test_id UUID NOT NULL REFERENCES campaign_tests(id) ON DELETE CASCADE,
    environment_id   UUID REFERENCES runtime_environments(id) ON DELETE SET NULL,
    environment_name TEXT,
    status           TEXT NOT NULL DEFAULT 'pending',
    defect_id        UUID REFERENCES defects(id) ON DELETE SET NULL,
    executed_by      UUID REFERENCES users(id) ON DELETE SET NULL,
    executed_at      TIMESTAMPTZ,
    updated_at       TIMESTAMPTZ NOT NULL,
    UNIQUE(campaign_id, campaign_test_id, environment_id)
);
CREATE INDEX IF NOT EXISTS idx_campaign_results_campaign_id ON campaign_results(campaign_id);
CREATE INDEX IF NOT EXISTS idx_campaign_results_campaign_test_id ON campaign_results(campaign_test_id);
CREATE INDEX IF NOT EXISTS idx_campaign_results_environment_id ON campaign_results(environment_id);
CREATE INDEX IF NOT EXISTS idx_campaign_results_defect_id ON campaign_results(defect_id);
-- The UNIQUE above treats NULLs as distinct, so it does not stop a second
-- result without an environment for the same test. Rows whose environment was
-- deleted keep its name and stay out of this index.
CREATE UNIQUE INDEX IF NOT EXISTS idx_campaign_results_no_env ON campaign_results(campaign_id, campaign_test_id) WHERE environment_id IS NULL AND environment_name IS NULL;

-- Campaign result events: immutable status change log (FR-042, UC-207)
-- campaign_id is denormalised so progress-over-time is one indexed scan without a join.
CREATE TABLE IF NOT EXISTS campaign_result_events (
    id          UUID PRIMARY KEY,
    result_id   UUID NOT NULL REFERENCES campaign_results(id) ON DELETE CASCADE,
    campaign_id UUID NOT NULL REFERENCES testing_campaigns(id) ON DELETE CASCADE,
    user_id     UUID REFERENCES users(id) ON DELETE SET NULL,
    from_status TEXT NOT NULL DEFAULT '',
    to_status   TEXT NOT NULL,
    defect_id   UUID REFERENCES defects(id) ON DELETE SET NULL,
    -- Automated runs (FR-409): where the verdict came from; empty for people.
    source      TEXT NOT NULL DEFAULT '',
    revision    TEXT NOT NULL DEFAULT '',
    branch      TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_campaign_result_events_result_id ON campaign_result_events(result_id);
CREATE INDEX IF NOT EXISTS idx_campaign_result_events_campaign ON campaign_result_events(campaign_id, created_at);
-- Serves the defect verification trail: find the results a defect ever touched.
CREATE INDEX IF NOT EXISTS idx_campaign_result_events_defect_id ON campaign_result_events(defect_id);

-- Requirement assignments: users assigned to requirements (FR-009)
CREATE TABLE IF NOT EXISTS requirement_assignments (
    id             UUID PRIMARY KEY,
    requirement_id UUID NOT NULL REFERENCES requirements(id) ON DELETE CASCADE,
    user_id        UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at     TIMESTAMPTZ NOT NULL,
    UNIQUE(requirement_id, user_id)
);
CREATE INDEX IF NOT EXISTS idx_requirement_assignments_requirement_id ON requirement_assignments(requirement_id);

-- Defect assignments: users assigned to defects (FR-009)
CREATE TABLE IF NOT EXISTS defect_assignments (
    id         UUID PRIMARY KEY,
    defect_id  UUID NOT NULL REFERENCES defects(id) ON DELETE CASCADE,
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL,
    UNIQUE(defect_id, user_id)
);
CREATE INDEX IF NOT EXISTS idx_defect_assignments_defect_id ON defect_assignments(defect_id);

-- Test case to requirement links (FR-016)
CREATE TABLE IF NOT EXISTS test_case_requirements (
    id             UUID PRIMARY KEY,
    test_case_id   UUID NOT NULL REFERENCES test_cases(id) ON DELETE CASCADE,
    requirement_id UUID NOT NULL REFERENCES requirements(id) ON DELETE CASCADE,
    link_type      TEXT NOT NULL DEFAULT 'requirement',
    created_at     TIMESTAMPTZ NOT NULL,
    UNIQUE(test_case_id, requirement_id)
);
CREATE INDEX IF NOT EXISTS idx_test_case_requirements_test_case_id ON test_case_requirements(test_case_id);
CREATE INDEX IF NOT EXISTS idx_test_case_requirements_requirement_id ON test_case_requirements(requirement_id);

-- Requirement to capability links (FR-005a)
CREATE TABLE IF NOT EXISTS requirement_capabilities (
    id             UUID PRIMARY KEY,
    requirement_id UUID NOT NULL REFERENCES requirements(id) ON DELETE CASCADE,
    capability_id  UUID NOT NULL REFERENCES capabilities(id) ON DELETE CASCADE,
    created_at     TIMESTAMPTZ NOT NULL,
    UNIQUE(requirement_id, capability_id)
);
CREATE INDEX IF NOT EXISTS idx_requirement_capabilities_requirement_id ON requirement_capabilities(requirement_id);
CREATE INDEX IF NOT EXISTS idx_requirement_capabilities_capability_id ON requirement_capabilities(capability_id);

-- Constraint to capability links
CREATE TABLE IF NOT EXISTS constraint_capabilities (
    id            UUID PRIMARY KEY,
    constraint_id UUID NOT NULL REFERENCES constraints(id) ON DELETE CASCADE,
    capability_id UUID NOT NULL REFERENCES capabilities(id) ON DELETE CASCADE,
    created_at    TIMESTAMPTZ NOT NULL,
    UNIQUE(constraint_id, capability_id)
);
CREATE INDEX IF NOT EXISTS idx_constraint_capabilities_constraint_id ON constraint_capabilities(constraint_id);
CREATE INDEX IF NOT EXISTS idx_constraint_capabilities_capability_id ON constraint_capabilities(capability_id);

-- Documents: long-form reference texts that define part of the product
-- (FR-044..FR-052). body is stored byte for byte; format says how it is shown
-- and exported. A document sits under a component or a capability, or
-- directly under the product.
CREATE TABLE IF NOT EXISTS documents (
    id               UUID PRIMARY KEY,
    product_id       UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    sequence_number  INTEGER NOT NULL DEFAULT 0,
    title            TEXT NOT NULL,
    body             TEXT NOT NULL DEFAULT '',
    format           TEXT NOT NULL DEFAULT 'markdown',
    state            TEXT NOT NULL DEFAULT 'draft',
    superseded_by_id UUID REFERENCES documents(id) ON DELETE SET NULL,
    component_id     UUID REFERENCES components(id) ON DELETE SET NULL,
    capability_id    UUID REFERENCES capabilities(id) ON DELETE SET NULL,
    milestone_id     UUID REFERENCES milestones(id) ON DELETE SET NULL,
    revision         INTEGER NOT NULL DEFAULT 1,
    created_at       TIMESTAMPTZ NOT NULL,
    updated_at       TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_documents_product_id ON documents(product_id);
CREATE INDEX IF NOT EXISTS idx_documents_superseded_by_id ON documents(superseded_by_id);
CREATE INDEX IF NOT EXISTS idx_documents_component_id ON documents(component_id);
CREATE INDEX IF NOT EXISTS idx_documents_capability_id ON documents(capability_id);
CREATE INDEX IF NOT EXISTS idx_documents_milestone_id ON documents(milestone_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_documents_product_seq ON documents(product_id, sequence_number);

-- Document revisions: a full snapshot per change of title, body or format
-- (FR-051). Like the audit trail, user_id has no foreign key so the history
-- outlives the account that wrote it.
CREATE TABLE IF NOT EXISTS document_revisions (
    id          UUID PRIMARY KEY,
    document_id UUID NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    revision    INTEGER NOT NULL,
    title       TEXT NOT NULL,
    body        TEXT NOT NULL DEFAULT '',
    format      TEXT NOT NULL DEFAULT 'markdown',
    user_id     UUID NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL,
    UNIQUE(document_id, revision)
);
CREATE INDEX IF NOT EXISTS idx_document_revisions_document_id ON document_revisions(document_id);

-- Document links (FR-048)
CREATE TABLE IF NOT EXISTS requirement_documents (
    id             UUID PRIMARY KEY,
    requirement_id UUID NOT NULL REFERENCES requirements(id) ON DELETE CASCADE,
    document_id    UUID NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    created_at     TIMESTAMPTZ NOT NULL,
    UNIQUE(requirement_id, document_id)
);
CREATE INDEX IF NOT EXISTS idx_requirement_documents_requirement_id ON requirement_documents(requirement_id);
CREATE INDEX IF NOT EXISTS idx_requirement_documents_document_id ON requirement_documents(document_id);

CREATE TABLE IF NOT EXISTS constraint_documents (
    id            UUID PRIMARY KEY,
    constraint_id UUID NOT NULL REFERENCES constraints(id) ON DELETE CASCADE,
    document_id   UUID NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    created_at    TIMESTAMPTZ NOT NULL,
    UNIQUE(constraint_id, document_id)
);
CREATE INDEX IF NOT EXISTS idx_constraint_documents_constraint_id ON constraint_documents(constraint_id);
CREATE INDEX IF NOT EXISTS idx_constraint_documents_document_id ON constraint_documents(document_id);

CREATE TABLE IF NOT EXISTS test_case_documents (
    id           UUID PRIMARY KEY,
    test_case_id UUID NOT NULL REFERENCES test_cases(id) ON DELETE CASCADE,
    document_id  UUID NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
    created_at   TIMESTAMPTZ NOT NULL,
    UNIQUE(test_case_id, document_id)
);
CREATE INDEX IF NOT EXISTS idx_test_case_documents_test_case_id ON test_case_documents(test_case_id);
CREATE INDEX IF NOT EXISTS idx_test_case_documents_document_id ON test_case_documents(document_id);

-- Defect links: polymorphic links to components, capabilities, test cases (FR-023)
CREATE TABLE IF NOT EXISTS defect_links (
    id          UUID PRIMARY KEY,
    defect_id   UUID NOT NULL REFERENCES defects(id) ON DELETE CASCADE,
    target_type TEXT NOT NULL,
    target_id   UUID NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL,
    UNIQUE(defect_id, target_type, target_id)
);
CREATE INDEX IF NOT EXISTS idx_defect_links_defect_id ON defect_links(defect_id);
CREATE INDEX IF NOT EXISTS idx_defect_links_target ON defect_links(target_type, target_id);

-- Comments: polymorphic (FR-026, FR-027)
CREATE TABLE IF NOT EXISTS comments (
    id         UUID PRIMARY KEY,
    product_id UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    owner_type TEXT NOT NULL,
    owner_id   UUID NOT NULL,
    -- NULL once the author's account is deleted: the thread keeps the comment.
    user_id    UUID REFERENCES users(id) ON DELETE SET NULL,
    body       TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_comments_owner ON comments(owner_type, owner_id);
CREATE INDEX IF NOT EXISTS idx_comments_product_id ON comments(product_id);

-- Attachments: polymorphic (FR-026, FR-027)
CREATE TABLE IF NOT EXISTS attachments (
    id           UUID PRIMARY KEY,
    product_id   UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    owner_type   TEXT NOT NULL,
    owner_id     UUID NOT NULL,
    -- NULL once the uploader's account is deleted: the file stays.
    user_id      UUID REFERENCES users(id) ON DELETE SET NULL,
    filename     TEXT NOT NULL,
    content_type TEXT NOT NULL DEFAULT '',
    size         INTEGER NOT NULL DEFAULT 0,
    storage_path TEXT NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_attachments_owner ON attachments(owner_type, owner_id);
CREATE INDEX IF NOT EXISTS idx_attachments_product_id ON attachments(product_id);

-- Audit trail: field-level change tracking (FR-031)
-- product_id has no foreign key on purpose: the history outlives the product,
-- so administrators can still read, export or purge it (admin audit page).
CREATE TABLE IF NOT EXISTS audit_trail (
    id          UUID PRIMARY KEY,
    product_id  UUID,
    entity_type TEXT NOT NULL,
    entity_id   UUID NOT NULL,
    user_id     UUID NOT NULL,
    action      TEXT NOT NULL,
    field_name  TEXT,
    old_value   TEXT,
    new_value   TEXT,
    created_at  TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_audit_trail_entity ON audit_trail(entity_type, entity_id);
CREATE INDEX IF NOT EXISTS idx_audit_trail_created_at ON audit_trail(created_at);
CREATE INDEX IF NOT EXISTS idx_audit_trail_product ON audit_trail(product_id, created_at);
