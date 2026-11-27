SET LOCAL lock_timeout = '3s';

ALTER TABLE assurance_resolution
  ADD COLUMN version bigint NOT NULL DEFAULT 1 CONSTRAINT assurance_resolution_version_js_safe CHECK (version BETWEEN -9007199254740991 AND 9007199254740991),
  ADD COLUMN updated_at timestamptz NOT NULL DEFAULT now();
ALTER TABLE forecast_call
  ADD COLUMN version bigint NOT NULL DEFAULT 1 CONSTRAINT forecast_call_version_js_safe CHECK (version BETWEEN -9007199254740991 AND 9007199254740991),
  ADD COLUMN updated_at timestamptz NOT NULL DEFAULT now();
ALTER TABLE forecast_contribution
  ADD COLUMN version bigint NOT NULL DEFAULT 1 CONSTRAINT forecast_contribution_version_js_safe CHECK (version BETWEEN -9007199254740991 AND 9007199254740991),
  ADD COLUMN updated_at timestamptz NOT NULL DEFAULT now();
ALTER TABLE forecast_snapshot
  ADD COLUMN version bigint NOT NULL DEFAULT 1 CONSTRAINT forecast_snapshot_version_js_safe CHECK (version BETWEEN -9007199254740991 AND 9007199254740991),
  ADD COLUMN updated_at timestamptz NOT NULL DEFAULT now();
ALTER TABLE sdr_handoff_reason
  ADD COLUMN version bigint NOT NULL DEFAULT 1 CONSTRAINT sdr_handoff_reason_version_js_safe CHECK (version BETWEEN -9007199254740991 AND 9007199254740991),
  ADD COLUMN updated_at timestamptz NOT NULL DEFAULT now();
ALTER TABLE weekly_plan
  ADD COLUMN version bigint NOT NULL DEFAULT 1 CONSTRAINT weekly_plan_version_js_safe CHECK (version BETWEEN -9007199254740991 AND 9007199254740991);

ALTER TABLE activity_review_response ADD COLUMN updated_at timestamptz NOT NULL DEFAULT now();
CREATE TRIGGER trg_activity_review_response_updated BEFORE UPDATE ON activity_review_response FOR EACH ROW EXECUTE FUNCTION set_updated_at();
ALTER TABLE role_assignment ADD COLUMN updated_at timestamptz NOT NULL DEFAULT now();
CREATE TRIGGER trg_role_assignment_updated BEFORE UPDATE ON role_assignment FOR EACH ROW EXECUTE FUNCTION set_updated_at();
ALTER TABLE team_membership ADD COLUMN updated_at timestamptz NOT NULL DEFAULT now();
CREATE TRIGGER trg_team_membership_updated BEFORE UPDATE ON team_membership FOR EACH ROW EXECUTE FUNCTION set_updated_at();
ALTER TABLE workspace ADD COLUMN updated_at timestamptz NOT NULL DEFAULT now();
CREATE TRIGGER trg_workspace_updated BEFORE UPDATE ON workspace FOR EACH ROW EXECUTE FUNCTION set_updated_at();
