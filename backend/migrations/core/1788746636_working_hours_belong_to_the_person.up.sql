-- Working hours are a person's own, on a clock that is also their own.
--
-- They were a pair of Go constants — 9 and 17 — read on a UTC instant, with no
-- way for anyone to change either. The obvious next step, an installation-wide
-- pair an admin sets, is the wrong shape: people on one team do not share
-- working hours, and one pair set for everybody is authoritatively wrong for
-- most of them while the people it fails have no recourse.
--
-- So they sit beside `locale`, which is the setting they are most like: each
-- person sets their own and nobody sets it for anybody else.
--
-- ABSENT MEANS NOBODY HAS CHOSEN, which is why every column here is nullable
-- and why `timezone` becomes nullable with them. It was NOT NULL DEFAULT 'UTC'
-- and nothing in the product ever wrote it, so every value it holds is that
-- default rather than a choice — and a scheduler reading them as choices is
-- exactly the defect this change is about: "9am" meant 4pm in Ho Chi Minh City
-- and a Monday morning in Saigon was still Sunday. The rows carrying the
-- default are set to NULL for that reason, and only they: a value that is not
-- 'UTC' could only have been written by hand, and is left alone.
--
-- docs/explanation/scheduling.md is the design this implements.
SET LOCAL lock_timeout = '3s';

ALTER TABLE app_user
    ALTER COLUMN timezone DROP NOT NULL,
    ALTER COLUMN timezone DROP DEFAULT,
    -- Minutes past local midnight rather than a `time`: the two are compared
    -- against a slot's own minute-of-day, and an integer compares without a
    -- date to attach the time to.
    ADD COLUMN work_start_minute smallint,
    ADD COLUMN work_end_minute smallint,
    -- ISO-8601 weekday numbers, 1 = Monday. An array rather than a bitmask
    -- because a person reading the row should be able to see which days they
    -- work without decoding anything.
    ADD COLUMN work_days smallint[],
    ADD CONSTRAINT app_user_work_start_minute_of_day
        CHECK (work_start_minute IS NULL OR work_start_minute BETWEEN 0 AND 1439),
    ADD CONSTRAINT app_user_work_end_minute_of_day
        CHECK (work_end_minute IS NULL OR work_end_minute BETWEEN 1 AND 1440),
    -- A range, so an end before its start is refused here rather than answered
    -- as an empty calendar somewhere downstream. Both or neither: half a range
    -- is not a narrower setting, it is an unreadable one.
    ADD CONSTRAINT app_user_work_range_is_a_range
        CHECK ((work_start_minute IS NULL) = (work_end_minute IS NULL)
               AND (work_start_minute IS NULL OR work_start_minute < work_end_minute)),
    ADD CONSTRAINT app_user_work_days_are_weekdays
        CHECK (work_days IS NULL
               OR (array_length(work_days, 1) BETWEEN 1 AND 7
                   AND work_days <@ ARRAY[1,2,3,4,5,6,7]::smallint[]));

UPDATE app_user SET timezone = NULL WHERE timezone = 'UTC';
