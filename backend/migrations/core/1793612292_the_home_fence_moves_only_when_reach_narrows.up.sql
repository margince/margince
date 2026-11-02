SET LOCAL lock_timeout = '3s';

-- The fence moved on writes that only widen reach, or that touch columns no
-- access check reads: capture filling in a participant's contact, a calendar
-- sync rewriting its guests. Every such write ended every reader's cached Home.
-- Each trigger now fires only on a change that can take a record out of
-- somebody's reach, in the direction that narrows it.

-- The receipt is held to the five minutes Home may lag for background work,
-- like the worklist, rather than ended by every machine audit row.
DROP TRIGGER home_read_receipt_fence ON audit_log;
DROP FUNCTION advance_home_receipt_fence();
CREATE OR REPLACE FUNCTION commit_home_read_fence() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  UPDATE home_read_fence SET generation = generation + 1 WHERE singleton;
  DELETE FROM home_read_fence_pending WHERE xact = NEW.xact AND fence = NEW.fence;
  RETURN NULL;
END;
$$;
ALTER TABLE home_read_cache DROP COLUMN receipt_fence;
ALTER TABLE home_read_fence DROP COLUMN receipt_generation;

DROP TRIGGER home_read_fence_record_grant ON record_grant;
DROP TRIGGER home_read_fence_deal_owner ON deal;
DROP TRIGGER home_read_fence_lead_owner ON lead;
DROP TRIGGER home_read_fence_project_owner ON project;
DROP TRIGGER home_read_fence_contact_reach ON contact;
DROP TRIGGER home_read_fence_company_reach ON company;
DROP TRIGGER home_read_fence_activity_reach ON activity;
DROP TRIGGER home_read_fence_audience_member ON activity_audience_member;
DROP TRIGGER home_read_fence_link_change ON activity_link;
DROP TRIGGER home_read_fence_participant ON activity_participant;
DROP TRIGGER home_read_fence_capture_import ON capture_import;
DROP TRIGGER home_read_fence_commitment_admission ON activity_commitment_admission;
DROP TRIGGER home_read_fence_company_domain ON company_domain;
DROP TRIGGER home_read_fence_workspace_domain ON workspace_email_domain;

-- A new share or audience member only widens.
CREATE TRIGGER home_read_fence_record_grant AFTER UPDATE OR DELETE ON record_grant
  FOR EACH STATEMENT EXECUTE FUNCTION advance_home_read_fence();
CREATE TRIGGER home_read_fence_audience_member AFTER UPDATE OR DELETE ON activity_audience_member
  FOR EACH STATEMENT EXECUTE FUNCTION advance_home_read_fence();

CREATE TRIGGER home_read_fence_deal_owner AFTER UPDATE ON deal FOR EACH ROW
  WHEN (OLD.owner_id IS DISTINCT FROM NEW.owner_id OR (OLD.archived_at IS NULL AND NEW.archived_at IS NOT NULL))
  EXECUTE FUNCTION advance_home_read_fence();
CREATE TRIGGER home_read_fence_lead_owner AFTER UPDATE ON lead FOR EACH ROW
  WHEN (OLD.owner_id IS DISTINCT FROM NEW.owner_id OR (OLD.archived_at IS NULL AND NEW.archived_at IS NOT NULL))
  EXECUTE FUNCTION advance_home_read_fence();
CREATE TRIGGER home_read_fence_project_owner AFTER UPDATE ON project FOR EACH ROW
  WHEN (OLD.owner_id IS DISTINCT FROM NEW.owner_id OR (OLD.archived_at IS NULL AND NEW.archived_at IS NOT NULL))
  EXECUTE FUNCTION advance_home_read_fence();
CREATE TRIGGER home_read_fence_contact_reach AFTER UPDATE ON contact FOR EACH ROW
  WHEN (OLD.owner_id IS DISTINCT FROM NEW.owner_id OR OLD.visibility IS DISTINCT FROM NEW.visibility
        OR (OLD.archived_at IS NULL AND NEW.archived_at IS NOT NULL))
  EXECUTE FUNCTION advance_home_read_fence();
CREATE TRIGGER home_read_fence_company_reach AFTER UPDATE ON company FOR EACH ROW
  WHEN (OLD.owner_id IS DISTINCT FROM NEW.owner_id OR OLD.visibility IS DISTINCT FROM NEW.visibility
        OR OLD.is_anchor IS DISTINCT FROM NEW.is_anchor
        OR (OLD.archived_at IS NULL AND NEW.archived_at IS NOT NULL))
  EXECUTE FUNCTION advance_home_read_fence();
CREATE TRIGGER home_read_fence_activity_reach AFTER UPDATE ON activity FOR EACH ROW
  WHEN (OLD.audience IS DISTINCT FROM NEW.audience
        OR (OLD.restricted_at IS NULL AND NEW.restricted_at IS NOT NULL)
        OR (OLD.archived_at IS NULL AND NEW.archived_at IS NOT NULL)
        OR (OLD.assignee_id IS NOT NULL AND OLD.assignee_id IS DISTINCT FROM NEW.assignee_id)
        OR (OLD.sent_by_colleague IS NOT NULL AND OLD.sent_by_colleague IS DISTINCT FROM NEW.sent_by_colleague)
        OR OLD.captured_by IS DISTINCT FROM NEW.captured_by
        OR OLD.source_system IS DISTINCT FROM NEW.source_system
        OR OLD.source_activity_id IS DISTINCT FROM NEW.source_activity_id)
  EXECUTE FUNCTION advance_home_read_fence();

-- A link moved to another record; a removed one is the statement below.
CREATE TRIGGER home_read_fence_link_change AFTER UPDATE ON activity_link FOR EACH ROW
  WHEN (OLD.activity_id IS DISTINCT FROM NEW.activity_id OR OLD.contact_id IS DISTINCT FROM NEW.contact_id
        OR OLD.company_id IS DISTINCT FROM NEW.company_id OR OLD.deal_id IS DISTINCT FROM NEW.deal_id
        OR OLD.lead_id IS DISTINCT FROM NEW.lead_id OR OLD.project_id IS DISTINCT FROM NEW.project_id)
  EXECUTE FUNCTION advance_home_read_fence();
CREATE TRIGGER home_read_fence_link_removed AFTER DELETE ON activity_link
  FOR EACH STATEMENT EXECUTE FUNCTION advance_home_read_fence();

-- Only a participant naming a seat grants anybody reach, and it stands while
-- it carries an address, a contact or a channel identity
-- (auth.ParticipantStandsSQL): filling one in widens, clearing one narrows.
CREATE TRIGGER home_read_fence_participant AFTER UPDATE ON activity_participant FOR EACH ROW
  WHEN (OLD.user_id IS NOT NULL
        AND (OLD.user_id IS DISTINCT FROM NEW.user_id OR OLD.activity_id IS DISTINCT FROM NEW.activity_id
             OR (OLD.address IS NOT NULL AND NEW.address IS NULL)
             OR (OLD.contact_id IS NOT NULL AND NEW.contact_id IS NULL)
             OR (OLD.channel_user_id IS NOT NULL AND NEW.channel_user_id IS NULL)))
  EXECUTE FUNCTION advance_home_read_fence();
CREATE TRIGGER home_read_fence_participant_removed AFTER DELETE ON activity_participant FOR EACH ROW
  WHEN (OLD.user_id IS NOT NULL) EXECUTE FUNCTION advance_home_read_fence();

CREATE TRIGGER home_read_fence_capture_import AFTER UPDATE ON capture_import FOR EACH ROW
  WHEN (OLD.user_id IS DISTINCT FROM NEW.user_id OR OLD.activity_id IS DISTINCT FROM NEW.activity_id)
  EXECUTE FUNCTION advance_home_read_fence();
CREATE TRIGGER home_read_fence_capture_import_removed AFTER DELETE ON capture_import
  FOR EACH STATEMENT EXECUTE FUNCTION advance_home_read_fence();

-- These rows withhold content (auth.CommitmentContentRetainedClause and the
-- renewal arm), so an unavailable or renewed admission narrows and a removed one
-- only widens.
CREATE TRIGGER home_read_fence_commitment_admission AFTER INSERT ON activity_commitment_admission FOR EACH ROW
  WHEN (NEW.outcome = 'source_unavailable' OR NEW.renewed_by IS NOT NULL)
  EXECUTE FUNCTION advance_home_read_fence();
CREATE TRIGGER home_read_fence_commitment_admission_change AFTER UPDATE ON activity_commitment_admission FOR EACH ROW
  WHEN (OLD.outcome IS DISTINCT FROM NEW.outcome OR OLD.renewed_by IS DISTINCT FROM NEW.renewed_by
        OR OLD.source_activity_id IS DISTINCT FROM NEW.source_activity_id OR OLD.locator IS DISTINCT FROM NEW.locator)
  EXECUTE FUNCTION advance_home_read_fence();

-- Own domains open the colleague-sender arm: a domain that stops being ours
-- narrows, a new one only widens.
CREATE TRIGGER home_read_fence_company_domain AFTER UPDATE ON company_domain FOR EACH ROW
  WHEN (OLD.domain_folded IS DISTINCT FROM NEW.domain_folded OR OLD.company_id IS DISTINCT FROM NEW.company_id
        OR (OLD.archived_at IS NULL AND NEW.archived_at IS NOT NULL))
  EXECUTE FUNCTION advance_home_read_fence();
CREATE TRIGGER home_read_fence_company_domain_removed AFTER DELETE ON company_domain
  FOR EACH STATEMENT EXECUTE FUNCTION advance_home_read_fence();
CREATE TRIGGER home_read_fence_workspace_domain AFTER UPDATE ON workspace_email_domain FOR EACH ROW
  WHEN (OLD.domain IS DISTINCT FROM NEW.domain OR (OLD.verified AND NOT NEW.verified))
  EXECUTE FUNCTION advance_home_read_fence();
CREATE TRIGGER home_read_fence_workspace_domain_removed AFTER DELETE ON workspace_email_domain
  FOR EACH STATEMENT EXECUTE FUNCTION advance_home_read_fence();
