-- Assignments end with the product membership (UR-171). Until now removing a
-- member, or assigning a user through the API, could leave users assigned to
-- items of products they are no member of; this removes those assignments
-- once. From here on the services keep the rule.
DELETE FROM requirement_assignments
WHERE NOT EXISTS (
    SELECT 1 FROM requirements r
    JOIN product_roles pr ON pr.product_id = r.product_id
    WHERE r.id = requirement_assignments.requirement_id
      AND pr.user_id = requirement_assignments.user_id
);

DELETE FROM defect_assignments
WHERE NOT EXISTS (
    SELECT 1 FROM defects d
    JOIN product_roles pr ON pr.product_id = d.product_id
    WHERE d.id = defect_assignments.defect_id
      AND pr.user_id = defect_assignments.user_id
);
