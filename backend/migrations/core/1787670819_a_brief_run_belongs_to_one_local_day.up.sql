-- The overnight pass makes "one brief per rep per local day" a claim the
-- database keeps, rather than one a read-then-insert hopes for.
--
-- Two writers reach this table at once by design: the boot pass that backfills
-- a missed night and the hourly dispatcher can both find the same rep past
-- their briefing hour with no run yet. Checking first and inserting second is
-- exactly the race that produces two runs for one morning, and a rep who opens
-- Home to two different "today"s has been told two different things about the
-- same day.
--
-- local_day is the calendar date in the INSTALLATION's reporting zone
-- (installation.timezone), not the server's and not the rep's display zone.
-- It is stored rather than derived because the zone is a setting an operator
-- may change: a date computed at write time records which morning the run was
-- assembled for, and re-deriving it later from a changed setting would silently
-- re-label runs that already exist.
--
-- Bounded, because every statement below blocks writers on a table this
-- migration did not create: without a timeout, one open transaction holding a
-- conflicting lock stalls every write to brief_run for as long as this is
-- willing to queue, which is forever.
SET LOCAL lock_timeout = '3s';

ALTER TABLE brief_run ADD COLUMN local_day date;

-- The backfill reads the installation's own zone, not UTC. Using UTC here would
-- give a deployed database a different day boundary from the one the new writer
-- uses, and the two disagree by a whole day for exactly the runs assembled
-- either side of local midnight: an installation in Asia/Ho_Chi_Minh would
-- label a 05:30 local run as the previous day, then have the job assemble a
-- SECOND run for the morning that run already was.
--
-- COALESCE, because an installation whose settings row is missing has no zone
-- to read and UTC is the registered default anyway — the same fallback
-- settings.Get would apply, spelled here because a migration cannot call it.
UPDATE brief_run
SET local_day = (generated_at AT TIME ZONE COALESCE(
        (SELECT value #>> '{}' FROM setting WHERE key = 'installation.timezone'), 'UTC'))::date
WHERE local_day IS NULL;

-- The rep's queue state — snoozed, acted, dismissed — moves to the run that
-- survives before anything is deleted.
--
-- This is the part that is NOT bookkeeping. briefCandidates suppresses a deal
-- by looking for a marked brief_item across ALL of the rep's previous runs, not
-- just the newest, so deleting an older run deletes the reason a deal is
-- staying out of her queue. Without this step a deal she snoozed until Friday
-- reappears tomorrow morning, and one she dismissed comes back having changed
-- nothing — the exact behaviour B-E05.13 exists to prevent.
--
-- Only rows the survivor does not already carry, and only marked ones: the
-- survivor's own state is newer and wins, and an unmarked item suppresses
-- nothing so moving it would just widen the queue's history for no reason.
-- uq_brief_item_run_rank forces a rank, and these are appended after whatever
-- the survivor already holds.
WITH survivor AS (
    SELECT DISTINCT ON (user_id, local_day) id, user_id, local_day
    FROM brief_run
    ORDER BY user_id, local_day, generated_at DESC, id DESC
), superseded AS (
    SELECT br.id AS run_id, s.id AS survivor_id
    FROM brief_run br
    JOIN survivor s ON s.user_id = br.user_id AND s.local_day = br.local_day
    WHERE br.id <> s.id
), moving AS (
    SELECT bi.id,
           sup.survivor_id,
           row_number() OVER (PARTITION BY sup.survivor_id ORDER BY bi.state_at, bi.id) AS offset_rank
    FROM brief_item bi
    JOIN superseded sup ON sup.run_id = bi.brief_run_id
    WHERE bi.state <> 'new'
      AND NOT EXISTS (
        SELECT 1 FROM brief_item kept
        WHERE kept.brief_run_id = sup.survivor_id AND kept.deal_id = bi.deal_id)
)
UPDATE brief_item bi
SET brief_run_id = m.survivor_id,
    rank = COALESCE((SELECT max(rank) FROM brief_item k WHERE k.brief_run_id = m.survivor_id), 0)
           + m.offset_rank
FROM moving m
WHERE bi.id = m.id;

-- Now the superseded runs go. The newest run for each rep-day is the one whose
-- queue reflects the most recent ranking, and it is what the read already
-- served; the marks that were on the others are on it by the step above, and
-- brief_item cascades with whatever is left.
DELETE FROM brief_run br
WHERE EXISTS (
    SELECT 1 FROM brief_run newer
    WHERE newer.user_id = br.user_id
      AND newer.local_day = br.local_day
      AND (newer.generated_at, newer.id) > (br.generated_at, br.id)
);

ALTER TABLE brief_run ALTER COLUMN local_day SET NOT NULL;

ALTER TABLE brief_run ADD CONSTRAINT uq_brief_run_user_day UNIQUE (user_id, local_day);

-- The day-filtered read (WHERE user_id = $1 AND local_day = $2) is served by
-- the unique index above; idx_brief_run_user stays for the ordering the
-- history reads still do.
