-- A captured meeting belongs to the people on it.
--
-- A calendar event arrives from one seat's connector without anybody choosing
-- to share it. Read as a workspace-shared note — which is what a link-less
-- activity is — one seat's private diary became readable by every account in
-- the installation, private dinners and a partner negotiation included.
--
-- Capture now holds such a meeting to its participants and opens it when
-- something files it against a record the reader can already see. This migration
-- brings the rows already in the database to that state, and lands the two
-- structures the new behaviour needs.
--
-- Hand-logged meetings are deliberately untouched: nothing captured them, they
-- were written by a person for the workspace, and they stay workspace-readable
-- like the notes they are. The capture_import test below is what draws that line.

SET LOCAL lock_timeout = '5s';

-- 1. The overlap constraint applies to BOOKED meetings only.
--
-- It is a double-booking guard for the scheduling doors: one host may not hold
-- two meetings in the same hour. Captured calendar rows were never subject to it
-- because host_user_id was always NULL, and stamping the host below would make
-- every pair of a seat's real back-to-back appointments collide — a 45-minute
-- gap between two genuine calendar entries is not a fault to refuse, it is a
-- Tuesday.
--
-- source_system IS NULL is what separates the two: both booking doors leave it
-- unset, and every captured row carries its connector's name. The guard keeps
-- working for the door it was written for.
ALTER TABLE activity
    DROP CONSTRAINT activity_meeting_no_overlap;

ALTER TABLE activity
    ADD CONSTRAINT activity_meeting_no_overlap
    EXCLUDE USING gist (
        host_user_id WITH =,
        tsrange(timezone('UTC'::text, occurred_at),
                (timezone('UTC'::text, occurred_at) + '01:00:00'::interval)) WITH &&)
    WHERE (kind = 'meeting'::text
           AND host_user_id IS NOT NULL
           AND archived_at IS NULL
           AND source_system IS NULL);

-- 2. Whose calendar each captured meeting came off.
--
-- capture_import.user_id is the AUTHENTICATED seat whose connector landed the
-- row — capture writes it from the principal, never from anything a record
-- claimed. It is preferred over the `from` participant because a meeting can
-- carry several internal parties and UPDATE FROM would pick among them
-- arbitrarily; the import row names exactly one seat per import.
--
-- A meeting several seats imported takes the earliest, which is the seat whose
-- sync actually created the row. This is a label saying whose calendar the row
-- came off, not a claim that they organized the invitation.
--
-- The captured_by suffix is the FALLBACK, for the rows that have no import row
-- at all: capture_import was backfilled only where that stamp ends in a uuid
-- (migration 1788151532), so an older bare `connector:gcal` row has none. Both
-- sources are written by capture from the authenticated principal, so neither is
-- a claim a record made about itself. A row that yields neither keeps a NULL
-- host, which reads honestly as "no calendar claims this meeting".
UPDATE activity a
   SET host_user_id = coalesce(
       (SELECT ci.user_id
          FROM capture_import ci
         WHERE ci.activity_id = a.id
         ORDER BY ci.imported_at, ci.id
         LIMIT 1),
       (SELECT u.id
          FROM app_user u
         WHERE u.id = substring(a.captured_by from '([0-9a-f-]{36})$')::uuid))
 WHERE a.kind = 'meeting'
   AND a.host_user_id IS NULL
   AND a.restricted_at IS NULL
   AND a.captured_by LIKE 'connector:%';

-- 3. A captured meeting that nothing files anywhere is held to its people.
--
-- The same state capture now writes at birth, applied to the rows already here.
-- audience_reason IS NULL is what keeps a JUDGED hold out of this: a row whose
-- audience somebody already narrowed carries a reason, and re-stamping it would
-- overwrite what a person or a verdict decided.
--
-- What keeps a HAND-LOGGED meeting out is the captured_by provenance, read
-- directly rather than through the presence of a capture_import row.
--
-- The import row is the wrong test and would fail short, which is the one way a
-- privacy migration must not be wrong. capture_import was backfilled only for
-- rows whose captured_by ENDS IN A UUID (migration 1788151532), so a meeting
-- carrying the older bare `connector:gcal` stamp has none — and testing for one
-- would leave exactly those rows workspace-readable while the migration reported
-- success. A hand-logged meeting carries `human:<uuid>` and is excluded by the
-- provenance test itself.
--
-- The activity_link test leaves an already-filed meeting open: it is reachable
-- through a record, which is the condition that lifts this hold anyway.
UPDATE activity a
   SET audience = 'participants',
       audience_reason = 'no_counterparty'
 WHERE a.kind = 'meeting'
   AND a.audience = 'workspace'
   AND a.audience_reason IS NULL
   AND a.restricted_at IS NULL
   AND a.captured_by LIKE 'connector:%'
   AND NOT EXISTS (SELECT 1 FROM activity_link l WHERE l.activity_id = a.id);

-- 4. A person an AGENT created is visible to its owner until something widens it.
--
-- A human typing a contact into the UI is publishing it to the workspace on
-- purpose. An agent minting one from a tool call is not that decision, and the
-- create_record tool runs without a human seeing the row first — which is how a
-- contact created from one seat's meeting became readable by everybody.
--
-- source = 'manual' and the agent stamp together name exactly that path. Rows
-- the verdict classifier already judged carry its own captured_by and are left
-- alone: that judgement is a decision about the sender, and this is not the
-- place to revisit it. owner_id IS NOT NULL is required because an 'owner' row
-- with no owner is readable by nobody at all.
UPDATE person
   SET visibility = 'owner'
 WHERE visibility = 'workspace'
   AND owner_id IS NOT NULL
   AND source = 'manual'
   AND captured_by ~ '^agent:[0-9a-f-]{36}$';

-- 5. Which meetings have had their attendees resolved under the current rule.
--
-- Capture binds an invited colleague to their user_id now, and an attendee
-- carried by address alone is not a member of the meeting's audience — so a
-- colleague who was IN a meeting cannot read it once the row is held. The repair
-- pass in compose re-reads the stored calendar originals and binds them.
--
-- It needs a marker of its own rather than the participant replay's. That one
-- records a completed PARSE, and these rows already have it: they were replayed
-- under the old rule, which read the attendee list and deliberately bound no
-- colleague from it. Collapsing the two would make "we parsed this" and "we
-- resolved its attendees" indistinguishable the next time the rule changes.
--
-- No workspace column and no policy, matching activity_participant_replay: one
-- installation holds one organization, and the pass runs inside the workspace
-- transaction that every capture write uses.
CREATE TABLE activity_meeting_attendee_repair (
    activity_id uuid NOT NULL,
    repaired_at timestamptz DEFAULT now() NOT NULL,
    outcome text NOT NULL,
    CONSTRAINT activity_meeting_attendee_repair_pkey PRIMARY KEY (activity_id),
    CONSTRAINT activity_meeting_attendee_repair_activity_fk
        FOREIGN KEY (activity_id) REFERENCES activity(id) ON DELETE CASCADE,
    CONSTRAINT activity_meeting_attendee_repair_outcome_check
        CHECK (outcome IN ('attendees', 'none', 'unreadable'))
);
