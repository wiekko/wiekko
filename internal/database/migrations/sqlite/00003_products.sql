-- Product: top-level entity (FR-001)
CREATE TABLE IF NOT EXISTS products (
    id              TEXT PRIMARY KEY,
    name            TEXT NOT NULL UNIQUE,
    description     TEXT NOT NULL DEFAULT '',
    estimation_mode TEXT NOT NULL DEFAULT 'disabled',
    workflow_mode   TEXT NOT NULL DEFAULT 'kanban',
    status_sync    INTEGER NOT NULL DEFAULT 1,
    created_at      TEXT NOT NULL,
    updated_at      TEXT NOT NULL
);

-- Product roles: relocated from 00002 with proper FK (SR-201..SR-206)
CREATE TABLE IF NOT EXISTS product_roles (
    id         TEXT PRIMARY KEY,
    user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    product_id TEXT NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    role       TEXT NOT NULL,
    created_at TEXT NOT NULL,
    UNIQUE(user_id, product_id)
);
CREATE INDEX IF NOT EXISTS idx_product_roles_user_id ON product_roles(user_id);
CREATE INDEX IF NOT EXISTS idx_product_roles_product_id ON product_roles(product_id);

-- Milestones: product roadmap (FR-011, FR-012)
CREATE TABLE IF NOT EXISTS milestones (
    id          TEXT PRIMARY KEY,
    product_id  TEXT NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    type        TEXT NOT NULL DEFAULT 'planning',
    due_date    TEXT,
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_milestones_product_id ON milestones(product_id);

-- Components: product decomposition (FR-002)
CREATE TABLE IF NOT EXISTS components (
    id           TEXT PRIMARY KEY,
    product_id   TEXT NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    name         TEXT NOT NULL,
    description  TEXT NOT NULL DEFAULT '',
    milestone_id TEXT REFERENCES milestones(id) ON DELETE SET NULL,
    created_at   TEXT NOT NULL,
    updated_at   TEXT NOT NULL,
    UNIQUE(product_id, name)
);
CREATE INDEX IF NOT EXISTS idx_components_product_id ON components(product_id);
CREATE INDEX IF NOT EXISTS idx_components_milestone_id ON components(milestone_id);

-- Capabilities: linked to components or top-level (FR-003, FR-004)
CREATE TABLE IF NOT EXISTS capabilities (
    id           TEXT PRIMARY KEY,
    product_id   TEXT NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    component_id TEXT REFERENCES components(id) ON DELETE SET NULL,
    name         TEXT NOT NULL,
    description  TEXT NOT NULL DEFAULT '',
    milestone_id TEXT REFERENCES milestones(id) ON DELETE SET NULL,
    created_at   TEXT NOT NULL,
    updated_at   TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_capabilities_product_id ON capabilities(product_id);
CREATE INDEX IF NOT EXISTS idx_capabilities_component_id ON capabilities(component_id);
CREATE INDEX IF NOT EXISTS idx_capabilities_milestone_id ON capabilities(milestone_id);

-- Configurable statuses for requirements and defects (FR-323)
CREATE TABLE IF NOT EXISTS statuses (
    id          TEXT PRIMARY KEY,
    product_id  TEXT NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    entity_type TEXT NOT NULL,
    name        TEXT NOT NULL,
    position    INTEGER NOT NULL DEFAULT 0,
    is_default  INTEGER NOT NULL DEFAULT 0,
    is_closed   INTEGER NOT NULL DEFAULT 0,
    -- Kanban: items in an active column are activated for work (FR-318, FR-323a)
    is_active   INTEGER NOT NULL DEFAULT 0,
    created_at  TEXT NOT NULL,
    UNIQUE(product_id, entity_type, name)
);
CREATE INDEX IF NOT EXISTS idx_statuses_product_id ON statuses(product_id, entity_type);

-- Configurable categories for requirements and constraints
CREATE TABLE IF NOT EXISTS categories (
    id          TEXT PRIMARY KEY,
    product_id  TEXT NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    entity_type TEXT NOT NULL,
    name        TEXT NOT NULL,
    prefix      TEXT NOT NULL,
    position    INTEGER NOT NULL DEFAULT 0,
    created_at  TEXT NOT NULL,
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
    id           TEXT PRIMARY KEY,
    product_id   TEXT NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    name         TEXT NOT NULL,
    milestone_id TEXT REFERENCES milestones(id) ON DELETE SET NULL,
    status       TEXT NOT NULL DEFAULT 'planned',
    start_date   TEXT NOT NULL,
    end_date     TEXT NOT NULL,
    created_at   TEXT NOT NULL,
    updated_at   TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_sprints_product_id ON sprints(product_id);
CREATE INDEX IF NOT EXISTS idx_sprints_milestone_id ON sprints(milestone_id);

-- Requirements: backlog items (FR-005..FR-010, FR-013)
CREATE TABLE IF NOT EXISTS requirements (
    id                 TEXT PRIMARY KEY,
    product_id         TEXT NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    kind               TEXT NOT NULL DEFAULT 'requirement',
    parent_id          TEXT REFERENCES requirements(id) ON DELETE SET NULL,
    category_id        TEXT NOT NULL REFERENCES categories(id) ON DELETE RESTRICT,
    title              TEXT NOT NULL,
    description        TEXT NOT NULL DEFAULT '',
    definition_of_done TEXT NOT NULL DEFAULT '',
    status_id          TEXT REFERENCES statuses(id) ON DELETE SET NULL,
    milestone_id       TEXT REFERENCES milestones(id) ON DELETE SET NULL,
    sprint_id          TEXT REFERENCES sprints(id) ON DELETE SET NULL,
    estimation         TEXT,
    priority           INTEGER NOT NULL DEFAULT 0,
    sequence_number    INTEGER NOT NULL DEFAULT 0,
    is_coarse          INTEGER NOT NULL DEFAULT 0,
    is_active          INTEGER NOT NULL DEFAULT 0,
    created_at         TEXT NOT NULL,
    updated_at         TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_requirements_product_id ON requirements(product_id);
CREATE INDEX IF NOT EXISTS idx_requirements_kind ON requirements(product_id, kind);
CREATE INDEX IF NOT EXISTS idx_requirements_parent_id ON requirements(parent_id);
CREATE INDEX IF NOT EXISTS idx_requirements_category_id ON requirements(category_id);
CREATE INDEX IF NOT EXISTS idx_requirements_status_id ON requirements(status_id);
CREATE INDEX IF NOT EXISTS idx_requirements_milestone_id ON requirements(milestone_id);
CREATE INDEX IF NOT EXISTS idx_requirements_sprint_id ON requirements(sprint_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_requirements_product_seq ON requirements(product_id, sequence_number);

-- Runtime environments: freely definable labels (FR-019, FR-020).
-- Defined before defects, which reference an environment.
CREATE TABLE IF NOT EXISTS runtime_environments (
    id         TEXT PRIMARY KEY,
    product_id TEXT NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    created_at TEXT NOT NULL,
    UNIQUE(product_id, name)
);
CREATE INDEX IF NOT EXISTS idx_runtime_environments_product_id ON runtime_environments(product_id);

-- Defects: bugs and issues (FR-022..FR-025)
-- environment_id records where the defect was observed ("login fails in Firefox").
-- Unlike campaign_results it clears rather than cascades: a defect outlives the
-- environment label it was found in.
CREATE TABLE IF NOT EXISTS defects (
    id             TEXT PRIMARY KEY,
    product_id     TEXT NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    title          TEXT NOT NULL,
    description    TEXT NOT NULL DEFAULT '',
    criticality    TEXT NOT NULL DEFAULT 'medium',
    status_id      TEXT REFERENCES statuses(id) ON DELETE SET NULL,
    milestone_id   TEXT REFERENCES milestones(id) ON DELETE SET NULL,
    sprint_id      TEXT REFERENCES sprints(id) ON DELETE SET NULL,
    environment_id TEXT REFERENCES runtime_environments(id) ON DELETE SET NULL,
    estimation     TEXT,
    priority       INTEGER NOT NULL DEFAULT 0,
    is_active      INTEGER NOT NULL DEFAULT 0,
    created_at     TEXT NOT NULL,
    updated_at     TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_defects_product_id ON defects(product_id);
CREATE INDEX IF NOT EXISTS idx_defects_status_id ON defects(status_id);
CREATE INDEX IF NOT EXISTS idx_defects_milestone_id ON defects(milestone_id);
CREATE INDEX IF NOT EXISTS idx_defects_sprint_id ON defects(sprint_id);
CREATE INDEX IF NOT EXISTS idx_defects_environment_id ON defects(environment_id);

-- Constraints: non-functional requirements (architecture, security, UX rules)
CREATE TABLE IF NOT EXISTS constraints (
    id               TEXT PRIMARY KEY,
    product_id       TEXT NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    category_id      TEXT NOT NULL REFERENCES categories(id) ON DELETE RESTRICT,
    sequence_number  INTEGER NOT NULL DEFAULT 0,
    title            TEXT NOT NULL,
    description      TEXT NOT NULL DEFAULT '',
    compliance_state TEXT NOT NULL DEFAULT 'not_assessed',
    milestone_id     TEXT REFERENCES milestones(id) ON DELETE SET NULL,
    priority         INTEGER NOT NULL DEFAULT 0,
    created_at       TEXT NOT NULL,
    updated_at       TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_constraints_product_id ON constraints(product_id);
CREATE INDEX IF NOT EXISTS idx_constraints_category_id ON constraints(category_id);
CREATE INDEX IF NOT EXISTS idx_constraints_milestone_id ON constraints(milestone_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_constraints_product_seq ON constraints(product_id, sequence_number);

-- Pyramid levels: test categorization hierarchy (FR-032..FR-035)
CREATE TABLE IF NOT EXISTS pyramid_levels (
    id         TEXT PRIMARY KEY,
    product_id TEXT NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    parent_id  TEXT REFERENCES pyramid_levels(id) ON DELETE SET NULL,
    name       TEXT NOT NULL,
    position   INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL,
    UNIQUE(product_id, name)
);
CREATE INDEX IF NOT EXISTS idx_pyramid_levels_product_id ON pyramid_levels(product_id);

-- Test cases (FR-014..FR-018, FR-032)
CREATE TABLE IF NOT EXISTS test_cases (
    id               TEXT PRIMARY KEY,
    product_id       TEXT NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    component_id     TEXT REFERENCES components(id) ON DELETE SET NULL,
    capability_id    TEXT REFERENCES capabilities(id) ON DELETE SET NULL,
    title            TEXT NOT NULL,
    description      TEXT NOT NULL DEFAULT '',
    execution_mode   TEXT NOT NULL DEFAULT 'manual',
    test_identifier  TEXT,
    pyramid_level_id TEXT REFERENCES pyramid_levels(id) ON DELETE SET NULL,
    created_at       TEXT NOT NULL,
    updated_at       TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_test_cases_product_id ON test_cases(product_id);
CREATE INDEX IF NOT EXISTS idx_test_cases_pyramid_level_id ON test_cases(pyramid_level_id);

-- Test case environment matrix (FR-019)
CREATE TABLE IF NOT EXISTS test_case_environments (
    id             TEXT PRIMARY KEY,
    test_case_id   TEXT NOT NULL REFERENCES test_cases(id) ON DELETE CASCADE,
    environment_id TEXT NOT NULL REFERENCES runtime_environments(id) ON DELETE CASCADE,
    UNIQUE(test_case_id, environment_id)
);
CREATE INDEX IF NOT EXISTS idx_test_case_environments_test_case_id ON test_case_environments(test_case_id);

-- Testing campaigns (FR-320, FR-322, FR-332)
CREATE TABLE IF NOT EXISTS testing_campaigns (
    id          TEXT PRIMARY KEY,
    product_id  TEXT NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    status      TEXT NOT NULL DEFAULT 'draft',
    starts_at   TEXT,
    ends_at     TEXT,
    closed_at   TEXT,
    closed_by   TEXT REFERENCES users(id) ON DELETE SET NULL,
    created_at  TEXT NOT NULL,
    updated_at  TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_testing_campaigns_product_id ON testing_campaigns(product_id);
-- At most one active campaign per product (FR-322)
CREATE UNIQUE INDEX IF NOT EXISTS idx_testing_campaigns_active ON testing_campaigns(product_id) WHERE status = 'active';

-- Campaign results: status per campaign x test case x environment (FR-021)
-- environment_id cascades: on environment deletion the matrix rows must disappear, not
-- collapse into NULL rows that the UNIQUE constraint below cannot deduplicate.
CREATE TABLE IF NOT EXISTS campaign_results (
    id             TEXT PRIMARY KEY,
    campaign_id    TEXT NOT NULL REFERENCES testing_campaigns(id) ON DELETE CASCADE,
    test_case_id   TEXT NOT NULL REFERENCES test_cases(id) ON DELETE CASCADE,
    environment_id TEXT REFERENCES runtime_environments(id) ON DELETE CASCADE,
    status         TEXT NOT NULL DEFAULT 'pending',
    defect_id      TEXT REFERENCES defects(id) ON DELETE SET NULL,
    executed_by    TEXT REFERENCES users(id) ON DELETE SET NULL,
    executed_at    TEXT,
    updated_at     TEXT NOT NULL,
    UNIQUE(campaign_id, test_case_id, environment_id)
);
CREATE INDEX IF NOT EXISTS idx_campaign_results_campaign_id ON campaign_results(campaign_id);
CREATE INDEX IF NOT EXISTS idx_campaign_results_test_case_id ON campaign_results(test_case_id);
CREATE INDEX IF NOT EXISTS idx_campaign_results_defect_id ON campaign_results(defect_id);

-- Campaign result events: immutable status change log (FR-042, UC-207)
-- campaign_id is denormalised so progress-over-time is one indexed scan without a join.
CREATE TABLE IF NOT EXISTS campaign_result_events (
    id          TEXT PRIMARY KEY,
    result_id   TEXT NOT NULL REFERENCES campaign_results(id) ON DELETE CASCADE,
    campaign_id TEXT NOT NULL REFERENCES testing_campaigns(id) ON DELETE CASCADE,
    user_id     TEXT REFERENCES users(id) ON DELETE SET NULL,
    from_status TEXT NOT NULL DEFAULT '',
    to_status   TEXT NOT NULL,
    defect_id   TEXT REFERENCES defects(id) ON DELETE SET NULL,
    -- Automated runs (FR-409): where the verdict came from; empty for people.
    source      TEXT NOT NULL DEFAULT '',
    revision    TEXT NOT NULL DEFAULT '',
    branch      TEXT NOT NULL DEFAULT '',
    created_at  TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_campaign_result_events_result_id ON campaign_result_events(result_id);
CREATE INDEX IF NOT EXISTS idx_campaign_result_events_campaign ON campaign_result_events(campaign_id, created_at);
-- Serves the defect verification trail: find the results a defect ever touched.
CREATE INDEX IF NOT EXISTS idx_campaign_result_events_defect_id ON campaign_result_events(defect_id);

-- Requirement assignments: users assigned to requirements (FR-009)
CREATE TABLE IF NOT EXISTS requirement_assignments (
    id             TEXT PRIMARY KEY,
    requirement_id TEXT NOT NULL REFERENCES requirements(id) ON DELETE CASCADE,
    user_id        TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at     TEXT NOT NULL,
    UNIQUE(requirement_id, user_id)
);
CREATE INDEX IF NOT EXISTS idx_requirement_assignments_requirement_id ON requirement_assignments(requirement_id);

-- Defect assignments: users assigned to defects (FR-009)
CREATE TABLE IF NOT EXISTS defect_assignments (
    id         TEXT PRIMARY KEY,
    defect_id  TEXT NOT NULL REFERENCES defects(id) ON DELETE CASCADE,
    user_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TEXT NOT NULL,
    UNIQUE(defect_id, user_id)
);
CREATE INDEX IF NOT EXISTS idx_defect_assignments_defect_id ON defect_assignments(defect_id);

-- Test case to requirement links (FR-016)
CREATE TABLE IF NOT EXISTS test_case_requirements (
    id             TEXT PRIMARY KEY,
    test_case_id   TEXT NOT NULL REFERENCES test_cases(id) ON DELETE CASCADE,
    requirement_id TEXT NOT NULL REFERENCES requirements(id) ON DELETE CASCADE,
    link_type      TEXT NOT NULL DEFAULT 'requirement',
    created_at     TEXT NOT NULL,
    UNIQUE(test_case_id, requirement_id)
);
CREATE INDEX IF NOT EXISTS idx_test_case_requirements_test_case_id ON test_case_requirements(test_case_id);
CREATE INDEX IF NOT EXISTS idx_test_case_requirements_requirement_id ON test_case_requirements(requirement_id);

-- Requirement to capability links (FR-005a)
CREATE TABLE IF NOT EXISTS requirement_capabilities (
    id             TEXT PRIMARY KEY,
    requirement_id TEXT NOT NULL REFERENCES requirements(id) ON DELETE CASCADE,
    capability_id  TEXT NOT NULL REFERENCES capabilities(id) ON DELETE CASCADE,
    created_at     TEXT NOT NULL,
    UNIQUE(requirement_id, capability_id)
);
CREATE INDEX IF NOT EXISTS idx_requirement_capabilities_requirement_id ON requirement_capabilities(requirement_id);
CREATE INDEX IF NOT EXISTS idx_requirement_capabilities_capability_id ON requirement_capabilities(capability_id);

-- Constraint to capability links
CREATE TABLE IF NOT EXISTS constraint_capabilities (
    id            TEXT PRIMARY KEY,
    constraint_id TEXT NOT NULL REFERENCES constraints(id) ON DELETE CASCADE,
    capability_id TEXT NOT NULL REFERENCES capabilities(id) ON DELETE CASCADE,
    created_at    TEXT NOT NULL,
    UNIQUE(constraint_id, capability_id)
);
CREATE INDEX IF NOT EXISTS idx_constraint_capabilities_constraint_id ON constraint_capabilities(constraint_id);
CREATE INDEX IF NOT EXISTS idx_constraint_capabilities_capability_id ON constraint_capabilities(capability_id);

-- Defect links: polymorphic links to components, capabilities, test cases (FR-023)
CREATE TABLE IF NOT EXISTS defect_links (
    id          TEXT PRIMARY KEY,
    defect_id   TEXT NOT NULL REFERENCES defects(id) ON DELETE CASCADE,
    target_type TEXT NOT NULL,
    target_id   TEXT NOT NULL,
    created_at  TEXT NOT NULL,
    UNIQUE(defect_id, target_type, target_id)
);
CREATE INDEX IF NOT EXISTS idx_defect_links_defect_id ON defect_links(defect_id);
CREATE INDEX IF NOT EXISTS idx_defect_links_target ON defect_links(target_type, target_id);

-- Comments: polymorphic (FR-026, FR-027)
CREATE TABLE IF NOT EXISTS comments (
    id         TEXT PRIMARY KEY,
    product_id TEXT NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    owner_type TEXT NOT NULL,
    owner_id   TEXT NOT NULL,
    user_id    TEXT NOT NULL REFERENCES users(id),
    body       TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_comments_owner ON comments(owner_type, owner_id);
CREATE INDEX IF NOT EXISTS idx_comments_product_id ON comments(product_id);

-- Attachments: polymorphic (FR-026, FR-027)
CREATE TABLE IF NOT EXISTS attachments (
    id           TEXT PRIMARY KEY,
    product_id   TEXT NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    owner_type   TEXT NOT NULL,
    owner_id     TEXT NOT NULL,
    user_id      TEXT NOT NULL REFERENCES users(id),
    filename     TEXT NOT NULL,
    content_type TEXT NOT NULL DEFAULT '',
    size         INTEGER NOT NULL DEFAULT 0,
    storage_path TEXT NOT NULL,
    created_at   TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_attachments_owner ON attachments(owner_type, owner_id);
CREATE INDEX IF NOT EXISTS idx_attachments_product_id ON attachments(product_id);

-- Audit trail: field-level change tracking (FR-031)
CREATE TABLE IF NOT EXISTS audit_trail (
    id          TEXT PRIMARY KEY,
    entity_type TEXT NOT NULL,
    entity_id   TEXT NOT NULL,
    user_id     TEXT NOT NULL,
    action      TEXT NOT NULL,
    field_name  TEXT,
    old_value   TEXT,
    new_value   TEXT,
    created_at  TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_audit_trail_entity ON audit_trail(entity_type, entity_id);
CREATE INDEX IF NOT EXISTS idx_audit_trail_created_at ON audit_trail(created_at);
