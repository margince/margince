SET LOCAL lock_timeout = '5s';

ALTER TABLE activity ADD COLUMN meeting_notes text;
ALTER TABLE activity ADD CONSTRAINT activity_meeting_notes_kind CHECK (meeting_notes IS NULL OR kind = 'meeting') NOT VALID;

CREATE OR REPLACE FUNCTION activity_refuse_restricted_mutation() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
  IF TG_OP = 'DELETE' THEN
    IF OLD.restricted_at IS NOT NULL THEN
      RAISE EXCEPTION 'activity % is restricted under a statutory retention obligation until %', OLD.id, OLD.restricted_until
        USING ERRCODE = 'check_violation',
              CONSTRAINT = 'activity_restricted_immutable';
    END IF;
    RETURN OLD;
  END IF;

  -- The stamp never changes once written — the class OR the timestamp saying
  -- when it was earned. Leaving the timestamp mutable would let a writer keep
  -- the class and move the date: the same rewriting of history with an extra
  -- step, on the field a supervisory authority would be shown.
  --
  -- The one exception is the declared undo of a project filing: the class
  -- clears whole from an unrestricted row that nothing still qualifies, and the
  -- declaration names this one activity.
  IF OLD.retention_class IS NOT NULL
     AND (NEW.retention_class IS DISTINCT FROM OLD.retention_class
          OR NEW.retention_class_at IS DISTINCT FROM OLD.retention_class_at)
     AND NOT (coalesce(current_setting('margince.project_filing_undo', true), '') = OLD.id::text
              AND NEW.retention_class IS NULL AND NEW.retention_class_at IS NULL
              AND OLD.restricted_at IS NULL AND NEW.restricted_at IS NULL
              AND NOT EXISTS (SELECT 1 FROM activity_retention_evidence e
                               WHERE e.activity_id = OLD.id)
              AND NOT EXISTS (SELECT 1 FROM activity_link l
                               WHERE l.activity_id = OLD.id AND l.entity_type = 'project')
              AND NOT EXISTS (SELECT 1 FROM activity_link hl
                               JOIN deal hd ON hd.id = hl.deal_id
                               WHERE hl.activity_id = OLD.id AND hl.entity_type = 'deal'
                                 AND (hd.status = 'won'
                                      OR EXISTS (SELECT 1 FROM offer o
                                                  WHERE o.deal_id = hd.id AND o.status <> 'draft')))
              AND NOT EXISTS (SELECT 1 FROM activity_link h
                               LEFT JOIN contact hp ON hp.id = h.contact_id
                               LEFT JOIN company hc ON hc.id = h.company_id
                               LEFT JOIN deal hdl ON hdl.id = h.deal_id
                               LEFT JOIN lead hld ON hld.id = h.lead_id
                               LEFT JOIN project hpj ON hpj.id = h.project_id
                               WHERE h.activity_id = OLD.id
                                 AND (coalesce(hp.legal_hold, false) OR coalesce(hc.legal_hold, false)
                                      OR coalesce(hdl.legal_hold, false) OR coalesce(hld.legal_hold, false)
                                      OR coalesce(hpj.legal_hold, false)))) THEN
    RAISE EXCEPTION 'activity % carries retention class % earned at %, which is stamped once and never re-derived', OLD.id, OLD.retention_class, OLD.retention_class_at
      USING ERRCODE = 'check_violation',
            CONSTRAINT = 'activity_retention_class_monotonic';
  END IF;

  -- A restriction is substantiated at the moment it is written. The table
  -- CHECK sees only this row, so it can require a class and no more; the
  -- evidence is in another table and this is the only place that can look.
  IF OLD.restricted_at IS NULL AND NEW.restricted_at IS NOT NULL
     AND NOT EXISTS (SELECT 1 FROM activity_retention_evidence e
                      WHERE e.activity_id = NEW.id) THEN
    RAISE EXCEPTION 'activity % cannot be restricted with no retention evidence recording what qualified it', NEW.id
      USING ERRCODE = 'check_violation',
            CONSTRAINT = 'activity_restriction_needs_evidence';
  END IF;

  -- A deadline already recorded never moves nearer. A pin or a re-restriction
  -- of a row that still carries its class may only extend it.
  IF OLD.restricted_until IS NOT NULL AND NEW.restricted_at IS NOT NULL
     AND NEW.restricted_until < OLD.restricted_until THEN
    RAISE EXCEPTION 'activity % is held until % and a statutory deadline never shortens', OLD.id, OLD.restricted_until
      USING ERRCODE = 'check_violation',
            CONSTRAINT = 'activity_restriction_never_shortens';
  END IF;

  IF OLD.restricted_at IS NOT NULL THEN
    -- Still restricted after the write: refused outright.
    IF NEW.restricted_at IS NOT NULL THEN
      RAISE EXCEPTION 'activity % is restricted under a statutory retention obligation until %', OLD.id, OLD.restricted_until
        USING ERRCODE = 'check_violation',
              CONSTRAINT = 'activity_restricted_immutable';
    END IF;

    -- Lifting: the content goes with it, in this statement. Both legitimate
    -- callers erase as they lift, and a lift that leaves the body readable is
    -- not a completion of the erasure — it is a way to undo the restriction
    -- and keep the data.
    IF NEW.body IS NOT NULL OR NEW.meeting_notes IS NOT NULL OR NEW.raw IS NOT NULL
       OR NEW.counterparty_email IS NOT NULL
       -- The byline a record was imported with names a human exactly as the
       -- body does. Cleared outright by every writer, so null is the test —
       -- unlike the subject below, which has a tombstone to be replaced by.
       OR NEW.source_author_name IS NOT NULL
       -- The subject must not survive AS IT WAS. It may go null or be replaced
       -- by the erasure's tombstone name — erasuretimeline.go writes the
       -- placeholder rather than a null, so demanding null here would refuse
       -- the very statement this guard exists to admit. The test is that it
       -- CHANGED, which the placeholder satisfies and keeping the original
       -- does not. Spelling the placeholder's literal value here would be a
       -- second copy of a Go constant, drifting the first time somebody edits
       -- one of them.
       OR (OLD.subject IS NOT NULL AND NEW.subject IS NOT DISTINCT FROM OLD.subject) THEN
      RAISE EXCEPTION 'activity % may only leave restriction by being erased: clear body, meeting_notes, raw, counterparty_email and source_author_name, and replace the subject, in the same statement', OLD.id
        USING ERRCODE = 'check_violation',
              CONSTRAINT = 'activity_restriction_lift_erases';
    END IF;
  END IF;

  RETURN NEW;
END;
$$;

