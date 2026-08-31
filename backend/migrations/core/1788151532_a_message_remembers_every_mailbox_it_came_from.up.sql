-- One row per mailbox that delivered a message, and the audience the row's
-- own decisions add up to.
--
-- An email is stored ONCE: activity identity is (source_system, source_id),
-- and source_id is the RFC822 Message-ID. So when the same message reaches two
-- seats' mailboxes there is one activity row, and captured_by names the first
-- importer alone. Every seat after the first left no trace at all.
--
-- That is fine while every seat wants the same thing from a message. It stops
-- being fine the moment one seat's mailbox holds mail back and another's does
-- not: with one row and one captured_by, the answer to "may colleagues read
-- this" depends on which sync happened to run first. A founder's confidential
-- thread that a colleague was cc'd on would be published by the colleague's
-- mailbox syncing first, and the founder's own posture would arrive too late
-- to matter.
--
-- capture_import gives each importing mailbox its own row to record what IT
-- decided, and activity.audience becomes DERIVED: the strictest answer across
-- every importer. Sync order stops mattering, because the strictest answer is
-- the same set whichever order it is computed in.

CREATE TABLE capture_import (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    activity_id uuid NOT NULL REFERENCES activity(id) ON DELETE CASCADE,
    -- RESTRICT, not CASCADE: this row is a seat's HOLD on a message, and
    -- cascading it away would let a later recompute widen mail that seat kept
    -- private. Nothing in this tree deletes an app_user (a seat is deactivated,
    -- which leaves the row), so the restriction costs nothing today — and if
    -- that ever changes, the delete fails loudly here instead of silently
    -- publishing the correspondence of somebody who has left.
    user_id uuid NOT NULL REFERENCES app_user(id) ON DELETE RESTRICT,
    -- What this seat's mailbox asked for at the moment the message landed, and
    -- what a classifier later concluded about it. Both are written by later
    -- work in this feature (the posture at import, the confidentiality
    -- verdict); this migration lands the columns so the recompute has one
    -- shape to read for the whole feature rather than a shape that changes
    -- under it. NULL means "nothing decided", which the recompute reads as
    -- the workspace floor and nothing stricter.
    posture_at_import text,
    verdict_status text,
    verdict_reason text,
    imported_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT capture_import_posture_check
        CHECK (posture_at_import IS NULL OR posture_at_import IN ('shared', 'classified', 'held')),
    CONSTRAINT capture_import_verdict_status_check
        CHECK (verdict_status IS NULL OR verdict_status IN
            ('pending', 'cleared', 'held', 'unsure', 'shared_by_owner', 'held_by_owner')),
    -- One import row per seat per message. A re-sync of the same message into
    -- the same mailbox is the same import, not a second one.
    CONSTRAINT capture_import_activity_user_key UNIQUE (activity_id, user_id)
);

-- The recompute's own read: every import row of one activity. The UNIQUE
-- constraint above leads with activity_id and serves it.
--
-- This index serves the other direction, which the purge and the per-owner
-- narrowing both walk: every message one seat imported.
CREATE INDEX capture_import_user_idx ON capture_import (user_id, activity_id);

-- Bounded wait: adding a column takes an ACCESS EXCLUSIVE lock on activity, and
-- a capture sync holding a row would otherwise queue every reader of the
-- timeline behind this statement for as long as that sync runs.
SET LOCAL lock_timeout = '3s';

-- Why the audience says what it says. Read by the owner on their own timeline
-- and withheld with the content from everyone else, so a colleague learns that
-- a message is held without learning that it is a termination letter.
--
-- The vocabulary is closed and grows with the feature: 'posture' (a mailbox
-- asked for it), 'workspace_floor' (the workspace turned mail sharing off),
-- 'no_record' (the message is filed under no record at all), 'pending_verdict'
-- (nothing has judged it yet), 'manual' (a human said so). The classifier's own
-- kinds arrive with the classifier.
ALTER TABLE activity ADD COLUMN audience_reason text;

COMMENT ON COLUMN activity.audience_reason IS
    'Why activity.audience is what it is. Withheld from a reader who may not read the content: the reason describes the content.';

-- Every activity that exists today was imported by exactly the seat named in
-- captured_by, which is the fact the whole feature needs and the only one this
-- backfill can honestly recover. captured_by is 'human:<uuid>' for a
-- hand-logged row and 'connector:<name>:<uuid>' for a captured one, so the
-- trailing uuid names the seat in both spellings.
--
-- Only CAPTURED rows get an import row. A hand-logged note was not imported
-- from anybody's mailbox, and giving it a capture_import row would hand its
-- author a per-owner hold on a record that never had one.
--
-- A seat who was cc'd on a message but whose own mailbox never synced it gets
-- no row, correctly: they did not import it. They read it as a participant.
--
-- A row whose captured_by carries no trailing uuid — a connection with no human
-- behind it stamps the bare connector id — gets no row either, because there is
-- no seat to recover and inventing one would attribute somebody's mail to a
-- person who never read it. Such a row keeps the audience it has for good: the
-- derivation refuses to move a row with no import rows at all, so it is never
-- widened on the strength of knowing nothing about it
-- (TestAMessageWithNoRecoverableImporterIsLeftAsItIs).
-- Every already-narrowed row gets a reason BEFORE it gets an import row, and
-- the reason is 'manual'.
--
-- This is the load-bearing statement of the whole migration. Before it, three
-- writers produced audience='participants' and none of them recorded why: the
-- capture ladder's link-less hold, the workspace mail-sharing floor, and a human
-- narrowing a message by hand. The derivation cannot rebuild any of the three,
-- and a narrowed row with no reason and an import row asking for nothing reads
-- to it as "no contributor wants this held" — so the next sync of any mailbox
-- that has the message would publish it, with an audit row and an event, and
-- nobody in the loop.
--
-- 'manual' is the honest word for all three: this system had no derived
-- narrowings at all, so every narrow row that exists is a decision somebody made
-- and nothing here may overturn. A row a human later re-opens gets its reason
-- rewritten by the endpoint that opens it.
UPDATE activity
   SET audience_reason = 'manual'
 WHERE audience <> 'workspace' AND audience_reason IS NULL;

INSERT INTO capture_import (activity_id, user_id, imported_at)
SELECT a.id,
       u.id,
       a.created_at
  FROM activity a
  JOIN app_user u
    ON u.id = substring(a.captured_by from '([0-9a-f-]{36})$')::uuid
 WHERE a.captured_by LIKE 'connector:%'
ON CONFLICT (activity_id, user_id) DO NOTHING;
