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
-- Backfilling existing rows in UTC is the honest answer for them: they were all
-- written by the on-demand refresh path, which had no notion of a local day at
-- all, and generated_at is the only fact about which morning they belong to.
-- A duplicate among them is possible — the old path allowed several runs a day
-- — so the unique index is built after collapsing them to the newest per day.
-- Bounded, because every statement below blocks writers on a table this
-- migration did not create: without a timeout, one open transaction holding a
-- conflicting lock stalls every write to brief_run for as long as this is
-- willing to queue, which is forever.
SET LOCAL lock_timeout = '3s';

ALTER TABLE brief_run ADD COLUMN local_day date;

UPDATE brief_run SET local_day = (generated_at AT TIME ZONE 'UTC')::date WHERE local_day IS NULL;

-- The historical duplicates go, keeping the newest run for each rep-day: it is
-- the one whose queue reflects the most recent ranking, and it is what
-- LatestRun already served. brief_item cascades with its run.
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
