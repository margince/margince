-- The 25-link ceiling holds against two writers at once.
--
-- Three writers test `count(*) < 25` on activity_link and then insert, and a
-- count read under READ COMMITTED is a snapshot: two repairs for two different
-- attendees of the same meeting each see 24 and each insert, and the meeting
-- ends at 26. `uq_activity_link` enforces duplicate identity, not a total, so
-- nothing catches it. A fourth writer applied no total at all — which ceiling
-- held depended on which arm ran.
--
-- So the ceiling moves to the one place every writer passes through.
--
-- FOR NO KEY UPDATE, deliberately unlike the SHARE its sibling trigger takes on
-- the same row. That one is about a link racing a re-kind, and says two links
-- onto one activity "are not in conflict and have no reason to wait for each
-- other". For a COUNT they are exactly in conflict: that is the race. This lock
-- makes link inserts onto ONE activity queue, which is what lets the count
-- below be true when the insert lands. It is NO KEY UPDATE rather than UPDATE
-- because the row is a parent other tables reference, and a child taking a
-- foreign key to it (FOR KEY SHARE) must not queue behind a link.
--
-- It RAISES rather than skipping. A caller naming links is already refused at
-- 26 by the request bound with a 422 against `links`, so reaching here means a
-- race — and a silently dropped link is a record the caller asked to file and
-- was told was filed. The sweeps that insert in batches keep their own count
-- guard, so they skip an over-full meeting rather than meeting this at all;
-- when they do race, the transaction aborts and the next tick sees the ceiling
-- and skips. Self-healing, and never a lie.
CREATE OR REPLACE FUNCTION activity_link_refuses_past_the_ceiling()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    filed integer;
BEGIN
    -- Locked before counted, for the reason the sibling trigger gives about
    -- order: a count taken before the lock is an answer from before whoever
    -- else is inserting, and waiting afterwards would not change it.
    PERFORM 1 FROM activity WHERE id = NEW.activity_id FOR NO KEY UPDATE;
    SELECT count(*) INTO filed FROM activity_link WHERE activity_id = NEW.activity_id;

    IF filed >= 25 THEN
        RAISE EXCEPTION
            'an activity may be filed under at most 25 records; this one already names %', filed
            USING ERRCODE = 'check_violation';
    END IF;

    RETURN NEW;
END;
$$;

CREATE TRIGGER activity_link_ceiling
    BEFORE INSERT ON activity_link
    FOR EACH ROW
    EXECUTE FUNCTION activity_link_refuses_past_the_ceiling();
