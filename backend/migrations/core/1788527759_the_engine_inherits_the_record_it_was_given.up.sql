-- The engine's own tables start empty, and the record they describe does not.
--
-- communication_basis and communication_suppression were added with the engine
-- (1788407500) and only ever written going forward. So an installation with
-- years of consent history handed the engine a blank sheet: every qualifying
-- event a rep recorded and every unsubscribe a subject clicked lived on in the
-- OLD tables, which the engine's suppression reader never consults.
--
-- This carries that history across. It invents nothing: no consent, no double
-- opt-in, no wording. Every row below is derived from a row somebody already
-- wrote, and anything that cannot be derived is left absent rather than guessed.

-- Qualifying events become the scoped basis they always were.
--
-- kind is subject_initiated_correspondence for all four source kinds, because
-- that is what a qualifying event MEANT: the subject did something that makes
-- answering them lawful. The old vocabulary (inbound_message, inquiry,
-- active_deal, in_person) described HOW we learned it, which survives in
-- source_activity_id and note rather than in the basis kind.
--
-- valid_until is occurred_at + 365 days, the reply window the engine already
-- applies to an unprompted follow-up (consent.defaultReplyWindow). A row older
-- than that is inserted ALREADY EXPIRED and authorizes nothing — which is the
-- honest carry, because the engine would not honour it if it were read live
-- either. Inserting it anyway keeps the history readable in a subject-access
-- export instead of silently dropping what the installation was told.
--
-- No thread_key. A qualifying event names an activity, not a conversation, and
-- a basis that claimed a thread it was not earned on would authorize every
-- message on that thread. Absent is correct: the reply arm reads the anchor.
INSERT INTO communication_basis
    (person_id, kind, source_activity_id, valid_from, valid_until, note, captured_by, captured_at)
SELECT
    qe.person_id,
    'subject_initiated_correspondence',
    -- Only an activity source survives as a source_activity_id. A 'deal' source
    -- is a real ground and NOT an activity, so it carries its provenance in the
    -- note instead of pointing the FK at a deal id that column cannot hold.
    CASE WHEN qe.source_entity_type = 'activity' THEN qe.source_entity_id END,
    qe.occurred_at,
    qe.occurred_at + interval '365 days',
    -- The provenance a reader needs to tell a carried row from a live one, in
    -- the column that already exists for prose. Not a ticket reference and not
    -- build narration: it says what this row was derived from, which is the
    -- question a data subject asks about it.
    'carried from the qualifying event recorded as ' || qe.kind
        || COALESCE(': ' || qe.note, ''),
    qe.captured_by,
    qe.created_at
FROM consent_qualifying_event qe
JOIN person p ON p.id = qe.person_id
WHERE
    -- An event whose source record is gone proves nothing a reader can check.
    -- The plan's own rule: no source, no basis - it stays unknown_legacy.
    (qe.source_entity_type IS DISTINCT FROM 'activity'
     OR EXISTS (SELECT 1 FROM activity a WHERE a.id = qe.source_entity_id))
    -- Idempotent, and this is the whole of it: re-running finds the row it
    -- wrote. Matched on the pair that identifies the carry rather than on a
    -- generated id, so a second run cannot double a person's history.
    AND NOT EXISTS (
        SELECT 1 FROM communication_basis cb
        WHERE cb.person_id = qe.person_id
          AND cb.kind = 'subject_initiated_correspondence'
          AND cb.captured_at = qe.created_at
    );

-- Withdrawals become marketing objections, and ONLY the marketing ones.
--
-- This filter is the whole safety of this migration, so it is stated rather
-- than implied. communication_suppression is NOT purpose-scoped: liveSuppression
-- reads the strongest live row for a person and applies it to every category,
-- and marketing_objection is in commsauthz.Absolute — no rollout mode softens
-- it, no evidence reaches past it.
--
-- So carrying every withdrawn person_consent row across would take somebody who
-- unsubscribed from one newsletter and block their invoices, their contract
-- notices and their security mail, permanently, with no way to see why. A
-- development database on this machine already holds a withdrawal against a
-- non-marketing purpose, so this is not a hypothetical shape.
--
-- Art. 21 is an objection to DIRECT MARKETING. The class filter is that rule.
INSERT INTO communication_suppression
    (person_id, kind, source, recorded_at, captured_by)
SELECT
    pc.person_id,
    'marketing_objection',
    'carried_from_person_consent',
    COALESCE(
        (SELECT ce.captured_at
           FROM consent_event ce
          WHERE ce.person_id = pc.person_id
            AND ce.purpose_id = pc.purpose_id
            AND ce.new_state = 'withdrawn'
          ORDER BY ce.captured_at DESC
          LIMIT 1),
        -- person_consent.captured_at is nullable, and a suppression with no
        -- recorded_at would sort last in liveSuppression's tie-break. now() is
        -- the honest floor: the objection is live NOW whenever it was made,
        -- and the proof row above carries the real moment where one exists.
        pc.captured_at, now()),
    COALESCE(
        (SELECT ce.captured_by
           FROM consent_event ce
          WHERE ce.person_id = pc.person_id
            AND ce.purpose_id = pc.purpose_id
            AND ce.new_state = 'withdrawn'
          ORDER BY ce.captured_at DESC
          LIMIT 1),
        'system:migration')
FROM person_consent pc
JOIN consent_purpose cp ON cp.id = pc.purpose_id
JOIN person p ON p.id = pc.person_id
WHERE pc.state = 'withdrawn'
  AND cp.class = 'marketing'
  -- One row per person, however many marketing purposes they withdrew from.
  -- The suppression is not purpose-scoped, so a second row would say the same
  -- thing twice and the reader takes LIMIT 1 anyway.
  AND NOT EXISTS (
      SELECT 1 FROM communication_suppression cs
      WHERE cs.person_id = pc.person_id
        AND cs.kind = 'marketing_objection'
  );

DO $$
DECLARE
    bases int;
    suppressions int;
BEGIN
    SELECT count(*) INTO bases FROM communication_basis
     WHERE note LIKE 'carried from the qualifying event%';
    SELECT count(*) INTO suppressions FROM communication_suppression
     WHERE source = 'carried_from_person_consent';
    -- Counts only. Naming a person here would put a consent state into a
    -- migration log, which is the one place nobody expects to find one.
    RAISE NOTICE 'the engine inherits % basis row(s) and % marketing objection(s)',
        bases, suppressions;
END $$;
