SET LOCAL lock_timeout = '3s';

-- One reader's last answer to one of Home's reads, served while the access
-- fence and the entry's own life both hold.
CREATE TABLE home_read_cache (
  reader_id   uuid        NOT NULL REFERENCES app_user(id) ON DELETE CASCADE,
  read        text        NOT NULL CHECK (read IN ('worklist', 'magic')),
  request_key text        NOT NULL,
  fence       bigint      NOT NULL,
  computed_at timestamptz NOT NULL,
  valid_until timestamptz NOT NULL,
  receipt_fence bigint    NOT NULL,
  walk_id     uuid,
  payload     jsonb       NOT NULL,
  PRIMARY KEY (reader_id, read, request_key)
);

-- When a reader last wrote. An answer computed before it is not theirs any
-- more, even one a read already in flight stores after the write.
CREATE TABLE home_read_forgotten (
  reader_id uuid        PRIMARY KEY REFERENCES app_user(id) ON DELETE CASCADE,
  at        timestamptz NOT NULL
);

-- Advances on every write that can take a record out of somebody's reach. A
-- cached answer records the generation its own snapshot read, so a change it
-- could not see invalidates it once committed.
-- receipt_generation advances on every transaction that writes a machine audit
-- row, which is what the receipt reports.
CREATE TABLE home_read_fence (
  singleton          boolean PRIMARY KEY DEFAULT true CHECK (singleton),
  generation         bigint  NOT NULL DEFAULT 0,
  receipt_generation bigint  NOT NULL DEFAULT 0
);
INSERT INTO home_read_fence DEFAULT VALUES;

-- A transaction that moved reach records itself here, and the counter advances
-- as it commits. Taking the counter's lock mid-transaction would order it
-- before locks the writer takes later, and two writers could deadlock on it;
-- taken last, at commit, it waits on nothing.
CREATE TABLE home_read_fence_pending (
  xact  xid8 NOT NULL,
  fence text NOT NULL CHECK (fence IN ('access', 'receipt')),
  PRIMARY KEY (xact, fence)
);

CREATE FUNCTION advance_home_read_fence_now(fence text) RETURNS void LANGUAGE plpgsql AS $$
BEGIN
  IF current_setting('margince.home_read_fence_' || fence, true) IS DISTINCT FROM 'on' THEN
    INSERT INTO home_read_fence_pending (xact, fence) VALUES (pg_current_xact_id(), fence) ON CONFLICT DO NOTHING;
    PERFORM set_config('margince.home_read_fence_' || fence, 'on', true);
  END IF;
END;
$$;

CREATE FUNCTION advance_home_read_fence() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  PERFORM advance_home_read_fence_now('access');
  RETURN NULL;
END;
$$;

-- A machine's audit row ends the receipt: it is a report of exactly those rows.
-- Ordered by commit, so a job that began before a receipt was read and commits
-- after it still ends it.
CREATE FUNCTION advance_home_receipt_fence() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF EXISTS (SELECT 1 FROM added_audit WHERE actor_type IN ('agent', 'system', 'connector')) THEN
    PERFORM advance_home_read_fence_now('receipt');
  END IF;
  RETURN NULL;
END;
$$;

CREATE FUNCTION commit_home_read_fence() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  UPDATE home_read_fence
     SET generation = generation + (NEW.fence = 'access')::int,
         receipt_generation = receipt_generation + (NEW.fence = 'receipt')::int
   WHERE singleton;
  DELETE FROM home_read_fence_pending WHERE xact = NEW.xact AND fence = NEW.fence;
  RETURN NULL;
END;
$$;

CREATE CONSTRAINT TRIGGER home_read_fence_at_commit AFTER INSERT ON home_read_fence_pending
  DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION commit_home_read_fence();

-- An activity with no link is discoverable by everyone, so its first link
-- narrows it. Links added beside existing ones, or to an activity created in
-- this transaction, only widen.
CREATE FUNCTION advance_home_read_fence_on_first_link() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF EXISTS (
    SELECT 1
      FROM (SELECT DISTINCT activity_id FROM added_links) n
      JOIN activity a ON a.id = n.activity_id AND a.created_at < now()
     WHERE (SELECT count(*) FROM activity_link l WHERE l.activity_id = n.activity_id)
         = (SELECT count(*) FROM added_links al WHERE al.activity_id = n.activity_id)
  ) THEN
    PERFORM advance_home_read_fence_now('access');
  END IF;
  RETURN NULL;
END;
$$;

-- Role, team, seat and field-mask changes already advance the reports fence.
CREATE TRIGGER home_read_fence_authority AFTER UPDATE ON report_projection_fence
  FOR EACH STATEMENT EXECUTE FUNCTION advance_home_read_fence();
CREATE TRIGGER home_read_fence_identity AFTER UPDATE OF merged_into_user_id, email_domain_folded ON app_user
  FOR EACH STATEMENT EXECUTE FUNCTION advance_home_read_fence();
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
CREATE TRIGGER home_read_fence_first_link AFTER INSERT ON activity_link
  REFERENCING NEW TABLE AS added_links
  FOR EACH STATEMENT EXECUTE FUNCTION advance_home_read_fence_on_first_link();
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

-- An erasure, anonymization or restriction: what it certified gone must not be
-- served from a copy.
CREATE TRIGGER home_read_fence_scrub AFTER INSERT ON audit_log FOR EACH ROW
  WHEN (NEW.action = ANY (ARRAY['erase', 'anonymize', 'restrict']))
  EXECUTE FUNCTION advance_home_read_fence();
CREATE TRIGGER home_read_receipt_fence AFTER INSERT ON audit_log
  REFERENCING NEW TABLE AS added_audit
  FOR EACH STATEMENT EXECUTE FUNCTION advance_home_receipt_fence();
