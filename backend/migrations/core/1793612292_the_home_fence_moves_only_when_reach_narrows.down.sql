SET LOCAL lock_timeout = '3s';

DROP TRIGGER home_read_fence_record_grant ON record_grant;
DROP TRIGGER home_read_fence_audience_member ON activity_audience_member;
DROP TRIGGER home_read_fence_deal_owner ON deal;
DROP TRIGGER home_read_fence_lead_owner ON lead;
DROP TRIGGER home_read_fence_project_owner ON project;
DROP TRIGGER home_read_fence_contact_reach ON contact;
DROP TRIGGER home_read_fence_company_reach ON company;
DROP TRIGGER home_read_fence_activity_reach ON activity;
DROP TRIGGER home_read_fence_link_change ON activity_link;
DROP TRIGGER home_read_fence_link_removed ON activity_link;
DROP TRIGGER home_read_fence_participant ON activity_participant;
DROP TRIGGER home_read_fence_participant_removed ON activity_participant;
DROP TRIGGER home_read_fence_capture_import ON capture_import;
DROP TRIGGER home_read_fence_capture_import_removed ON capture_import;
DROP TRIGGER home_read_fence_commitment_admission ON activity_commitment_admission;
DROP TRIGGER home_read_fence_commitment_admission_change ON activity_commitment_admission;
DROP TRIGGER home_read_fence_company_domain ON company_domain;
DROP TRIGGER home_read_fence_company_domain_removed ON company_domain;
DROP TRIGGER home_read_fence_workspace_domain ON workspace_email_domain;
DROP TRIGGER home_read_fence_workspace_domain_removed ON workspace_email_domain;

ALTER TABLE home_read_fence ADD COLUMN receipt_generation bigint NOT NULL DEFAULT 0;
ALTER TABLE home_read_cache ADD COLUMN receipt_fence bigint NOT NULL DEFAULT 0;
-- A kept receipt carries no receipt fence, so none can be told current. The
-- ALTER above holds the table, so no receipt lands between it and this.
DELETE FROM home_read_cache WHERE read = 'magic';
CREATE OR REPLACE FUNCTION commit_home_read_fence() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  UPDATE home_read_fence
     SET generation = generation + (NEW.fence = 'access')::int,
         receipt_generation = receipt_generation + (NEW.fence = 'receipt')::int
   WHERE singleton;
  DELETE FROM home_read_fence_pending WHERE xact = NEW.xact AND fence = NEW.fence;
  RETURN NULL;
END;
$$;
CREATE FUNCTION advance_home_receipt_fence() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF EXISTS (SELECT 1 FROM added_audit WHERE actor_type IN ('agent', 'system', 'connector')) THEN
    PERFORM advance_home_read_fence_now('receipt');
  END IF;
  RETURN NULL;
END;
$$;
CREATE TRIGGER home_read_receipt_fence AFTER INSERT ON audit_log
  REFERENCING NEW TABLE AS added_audit
  FOR EACH STATEMENT EXECUTE FUNCTION advance_home_receipt_fence();

CREATE TRIGGER home_read_fence_record_grant AFTER INSERT OR UPDATE OR DELETE ON record_grant
  FOR EACH STATEMENT EXECUTE FUNCTION advance_home_read_fence();
CREATE TRIGGER home_read_fence_deal_owner AFTER UPDATE ON deal FOR EACH ROW
  WHEN (OLD.owner_id IS DISTINCT FROM NEW.owner_id OR OLD.archived_at IS DISTINCT FROM NEW.archived_at)
  EXECUTE FUNCTION advance_home_read_fence();
CREATE TRIGGER home_read_fence_lead_owner AFTER UPDATE ON lead FOR EACH ROW
  WHEN (OLD.owner_id IS DISTINCT FROM NEW.owner_id OR OLD.archived_at IS DISTINCT FROM NEW.archived_at)
  EXECUTE FUNCTION advance_home_read_fence();
CREATE TRIGGER home_read_fence_project_owner AFTER UPDATE ON project FOR EACH ROW
  WHEN (OLD.owner_id IS DISTINCT FROM NEW.owner_id OR OLD.archived_at IS DISTINCT FROM NEW.archived_at)
  EXECUTE FUNCTION advance_home_read_fence();
CREATE TRIGGER home_read_fence_contact_reach AFTER UPDATE ON contact FOR EACH ROW
  WHEN (OLD.owner_id IS DISTINCT FROM NEW.owner_id OR OLD.visibility IS DISTINCT FROM NEW.visibility
        OR OLD.archived_at IS DISTINCT FROM NEW.archived_at)
  EXECUTE FUNCTION advance_home_read_fence();
CREATE TRIGGER home_read_fence_company_reach AFTER UPDATE ON company FOR EACH ROW
  WHEN (OLD.owner_id IS DISTINCT FROM NEW.owner_id OR OLD.visibility IS DISTINCT FROM NEW.visibility
        OR OLD.is_anchor IS DISTINCT FROM NEW.is_anchor OR OLD.archived_at IS DISTINCT FROM NEW.archived_at)
  EXECUTE FUNCTION advance_home_read_fence();
CREATE TRIGGER home_read_fence_activity_reach AFTER UPDATE ON activity FOR EACH ROW
  WHEN (OLD.audience IS DISTINCT FROM NEW.audience OR OLD.restricted_at IS DISTINCT FROM NEW.restricted_at
        OR OLD.archived_at IS DISTINCT FROM NEW.archived_at
        OR OLD.assignee_id IS DISTINCT FROM NEW.assignee_id OR OLD.captured_by IS DISTINCT FROM NEW.captured_by
        OR OLD.sent_by_colleague IS DISTINCT FROM NEW.sent_by_colleague
        OR OLD.source_system IS DISTINCT FROM NEW.source_system
        OR OLD.source_activity_id IS DISTINCT FROM NEW.source_activity_id)
  EXECUTE FUNCTION advance_home_read_fence();
CREATE TRIGGER home_read_fence_audience_member AFTER INSERT OR UPDATE OR DELETE ON activity_audience_member
  FOR EACH STATEMENT EXECUTE FUNCTION advance_home_read_fence();
CREATE TRIGGER home_read_fence_link_change AFTER UPDATE OR DELETE ON activity_link
  FOR EACH STATEMENT EXECUTE FUNCTION advance_home_read_fence();
CREATE TRIGGER home_read_fence_participant AFTER UPDATE OR DELETE ON activity_participant
  FOR EACH STATEMENT EXECUTE FUNCTION advance_home_read_fence();
CREATE TRIGGER home_read_fence_capture_import AFTER UPDATE OR DELETE ON capture_import
  FOR EACH STATEMENT EXECUTE FUNCTION advance_home_read_fence();
CREATE TRIGGER home_read_fence_commitment_admission AFTER UPDATE OR DELETE ON activity_commitment_admission
  FOR EACH STATEMENT EXECUTE FUNCTION advance_home_read_fence();
CREATE TRIGGER home_read_fence_company_domain AFTER UPDATE OR DELETE ON company_domain
  FOR EACH STATEMENT EXECUTE FUNCTION advance_home_read_fence();
CREATE TRIGGER home_read_fence_workspace_domain AFTER INSERT OR UPDATE OR DELETE ON workspace_email_domain
  FOR EACH STATEMENT EXECUTE FUNCTION advance_home_read_fence();
