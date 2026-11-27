SET LOCAL lock_timeout = '3s';

-- These rows are written once and never updated, so a counter or a last-write
-- time on them only repeats the insert. A set_updated_at trigger goes with its column.
ALTER TABLE assurance_resolution DROP COLUMN version, DROP COLUMN updated_at;
ALTER TABLE forecast_call DROP COLUMN version, DROP COLUMN updated_at;
ALTER TABLE forecast_contribution DROP COLUMN version, DROP COLUMN updated_at;
ALTER TABLE forecast_snapshot DROP COLUMN version, DROP COLUMN updated_at;
ALTER TABLE sdr_handoff_reason DROP COLUMN version, DROP COLUMN updated_at;

-- A weekly plan is updated under a row lock, and nothing moves or reads its counter.
ALTER TABLE weekly_plan DROP COLUMN version;

DROP TRIGGER trg_activity_review_response_updated ON activity_review_response;
ALTER TABLE activity_review_response DROP COLUMN updated_at;
DROP TRIGGER trg_role_assignment_updated ON role_assignment;
ALTER TABLE role_assignment DROP COLUMN updated_at;
DROP TRIGGER trg_team_membership_updated ON team_membership;
ALTER TABLE team_membership DROP COLUMN updated_at;
DROP TRIGGER trg_workspace_updated ON workspace;
ALTER TABLE workspace DROP COLUMN updated_at;
