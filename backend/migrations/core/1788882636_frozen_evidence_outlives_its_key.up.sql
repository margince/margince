-- Frozen evidence outlives the record it names; its uniqueness key must not.
--
-- uq_activity_retention_evidence keys a derived row on the record that
-- qualified it — (activity_id, deal_id, deal_name, project_id, project_name,
-- basis) NULLS NOT DISTINCT, so the arm a row does not use compares equal
-- instead of making every row unique. That equality is what makes the writers'
-- ON CONFLICT DO NOTHING idempotent.
--
-- Both id columns are ON DELETE SET NULL, because a deleted deal or project has
-- to leave the proof standing rather than take it along. Once cleared, the only
-- thing telling two rows on one activity apart is the frozen NAME. So an
-- activity filed in turn under two same-named projects, both later deleted,
-- collapses to one key and the SECOND delete is refused by a unique violation —
-- and the same shape has been true of the deal arm since the baseline. What
-- fails is the delete; the evidence is intact, which is why this was debt and
-- not an incident.
--
-- The key is left alone and the index stops covering rows whose reference has
-- been cleared. Uniqueness exists to make a WRITE idempotent, and that
-- narrowing is safe only while a cleared reference means the record is GONE.
-- Two things make it mean that, and both are new here: an insert must name the
-- record it is derived from, and a clearing update is refused while the record
-- still exists. A row that has lost its id is then never the row an ON CONFLICT
-- could have matched, so excluding it costs no idempotency and retires the
-- collision on both arms at once.
--
-- Without the second of those the narrowing would open a hole rather than close
-- one: the freeze trigger admitted ANY non-null-to-null update, so a hand
-- cleared row would leave the index and the next stamp for the same record
-- would insert beside it instead of being deduplicated.
--
-- `deal_id IS NOT NULL OR project_id IS NOT NULL` says "this row's own
-- reference is live" without naming a basis. are_derived_names_its_record gives
-- a derived row exactly one of the two names, and each name is required by its
-- id (are_deal_name_with_id, are_project_name_with_id), so the arm a row does
-- not use is already null on both of its columns.

SET LOCAL lock_timeout = '3s';

-- Written first, so the index below is never briefly the only thing standing
-- between a null-referenced insert and a duplicate it can no longer catch.
--
-- BEFORE INSERT only. A CHECK cannot say this: it would be re-evaluated when
-- the FK nulls the id, and would then refuse the very deletion this migration
-- is here to allow. The freeze trigger is left to its own subject — this one
-- asserts what a row must arrive with, not what may happen to it later.
CREATE OR REPLACE FUNCTION activity_retention_evidence_names_its_record() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
  IF NEW.basis <> 'controller_pin' AND NEW.deal_id IS NULL AND NEW.project_id IS NULL THEN
    RAISE EXCEPTION 'retention evidence on activity % is derived from a % that it does not name; the qualifying record''s id is what keeps two same-named records apart once either is deleted', NEW.activity_id, NEW.basis
      USING ERRCODE = 'check_violation',
            CONSTRAINT = 'activity_retention_evidence_names_its_record';
  END IF;
  RETURN NEW;
END;
$$;

CREATE TRIGGER activity_retention_evidence_names_its_record
    BEFORE INSERT ON activity_retention_evidence
    FOR EACH ROW EXECUTE FUNCTION activity_retention_evidence_names_its_record();

DROP INDEX uq_activity_retention_evidence;
CREATE UNIQUE INDEX uq_activity_retention_evidence
    ON activity_retention_evidence
    USING btree (activity_id, deal_id, deal_name, project_id, project_name, basis)
    NULLS NOT DISTINCT
    WHERE (basis <> 'controller_pin'::text
           AND (deal_id IS NOT NULL OR project_id IS NOT NULL));

CREATE OR REPLACE FUNCTION activity_retention_evidence_is_frozen() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
  -- A row goes only with the activity it substantiates, through the CASCADE.
  -- A direct delete is refused. The two are distinguishable because the
  -- CASCADE has already removed the parent by the time this fires, so the
  -- activity is gone exactly when the delete is legitimate.
  IF TG_OP = 'DELETE' THEN
    IF EXISTS (SELECT 1 FROM activity a WHERE a.id = OLD.activity_id) THEN
      RAISE EXCEPTION 'retention evidence % is frozen and is removed only with the activity it substantiates', OLD.id
        USING ERRCODE = 'check_violation',
              CONSTRAINT = 'activity_retention_evidence_frozen';
    END IF;
    RETURN OLD;
  END IF;

  IF NEW.activity_id     IS DISTINCT FROM OLD.activity_id
     OR NEW.basis        IS DISTINCT FROM OLD.basis
     OR NEW.qualified_at IS DISTINCT FROM OLD.qualified_at
     OR NEW.deal_name    IS DISTINCT FROM OLD.deal_name
     OR NEW.project_name IS DISTINCT FROM OLD.project_name
     OR NEW.decided_by_name IS DISTINCT FROM OLD.decided_by_name
     OR NEW.reason       IS DISTINCT FROM OLD.reason
     OR NEW.created_at   IS DISTINCT FROM OLD.created_at
     -- The reference may be cleared BY ITS FK, never repointed and never
     -- cleared by hand. ON DELETE SET NULL fires once the parent row is
     -- already gone, so "does the record still exist" is exactly what tells a
     -- statement somebody wrote from the FK doing its job — the same test the
     -- DELETE arm above applies to the activity.
     --
     -- This is not tidiness. The uniqueness index stops covering a row whose
     -- reference has been cleared, and that is only safe while clearing means
     -- "the record is gone": a hand-cleared row leaves the index quietly, and
     -- the next stamp for the same record inserts beside it instead of being
     -- deduplicated.
     --
     -- decided_by is left as it was. It is ON DELETE SET NULL to app_user and
     -- carries the same shape, but a pin is outside the index this protects,
     -- so tightening it here would be a claim about a different invariant.
     OR (NEW.deal_id IS NOT NULL AND NEW.deal_id IS DISTINCT FROM OLD.deal_id)
     OR (NEW.deal_id IS NULL AND OLD.deal_id IS NOT NULL
         AND EXISTS (SELECT 1 FROM deal d WHERE d.id = OLD.deal_id))
     OR (NEW.project_id IS NOT NULL AND NEW.project_id IS DISTINCT FROM OLD.project_id)
     OR (NEW.project_id IS NULL AND OLD.project_id IS NOT NULL
         AND EXISTS (SELECT 1 FROM project p WHERE p.id = OLD.project_id))
     OR (NEW.decided_by IS NOT NULL AND NEW.decided_by IS DISTINCT FROM OLD.decided_by) THEN
    RAISE EXCEPTION 'retention evidence % is frozen at the moment it qualified and may not be rewritten', OLD.id
      USING ERRCODE = 'check_violation',
            CONSTRAINT = 'activity_retention_evidence_frozen';
  END IF;

  RETURN NEW;
END;
$$;
