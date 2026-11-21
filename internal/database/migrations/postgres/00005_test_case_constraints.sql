-- Test case to constraint links (FR-150). A test that verifies a constraint,
-- or part of it, is evidence for the constraint's compliance.
CREATE TABLE IF NOT EXISTS test_case_constraints (
    id            UUID PRIMARY KEY,
    test_case_id  UUID NOT NULL REFERENCES test_cases(id) ON DELETE CASCADE,
    constraint_id UUID NOT NULL REFERENCES constraints(id) ON DELETE CASCADE,
    created_at    TIMESTAMPTZ NOT NULL,
    UNIQUE(test_case_id, constraint_id)
);
CREATE INDEX IF NOT EXISTS idx_test_case_constraints_test_case_id ON test_case_constraints(test_case_id);
CREATE INDEX IF NOT EXISTS idx_test_case_constraints_constraint_id ON test_case_constraints(constraint_id);
