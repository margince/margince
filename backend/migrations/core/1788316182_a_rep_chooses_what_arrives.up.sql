-- What a rep wants DELIVERED, as opposed to what they can open.
--
-- The morning brief and the weekly review are both on the screen already. The
-- mail is a NUDGE toward them, and a nudge nobody asked for is the product
-- deciding how often it may interrupt someone.
--
-- ON app_user rather than a table of its own, for the reason locale is there:
-- these are one person's settings about their own account, one row per seat,
-- read on the same query that resolves the seat. A table would be a join to
-- learn two words and a flag.
--
-- NULLABLE, and the nulls are the point. A member who has never chosen has no
-- value, which is NOT the same as choosing 'none': the first follows the
-- installation's default and moves if that default moves, the second is a
-- decision that stays. Writing a choice nobody made would freeze today's
-- default forever, which is the same ruling app_user.locale makes.
SET LOCAL lock_timeout = '5s';

ALTER TABLE app_user
    -- Whether the morning brief and the weekly review arrive by mail.
    ADD COLUMN morning_brief_delivery text,
    ADD COLUMN weekly_delivery text,

    -- Whether a day with nothing to act on still gets a message.
    --
    -- Its own setting rather than an implication of the two above: "tell me
    -- when there is something" and "tell me every morning either way" are
    -- different asks, and a rep who wants the first gets a quieter inbox than
    -- one who wants the second. Absent reads as no — a product that mails to
    -- say nothing happened has to be asked to.
    ADD COLUMN quiet_day_notice boolean,

    -- The local hour a delivery should not arrive before, in the installation's
    -- reporting zone. Absent means the job's own hour, which is what every seat
    -- gets today.
    ADD COLUMN delivery_hour_local int,

    -- The two delivery settings share ONE vocabulary, so a reader who learns
    -- one has learned the other. 'none' is a CHOICE to receive nothing, which
    -- NULL is not.
    ADD CONSTRAINT app_user_morning_brief_delivery_check CHECK (
        morning_brief_delivery IS NULL OR morning_brief_delivery IN ('none', 'email')),
    ADD CONSTRAINT app_user_weekly_delivery_check CHECK (
        weekly_delivery IS NULL OR weekly_delivery IN ('none', 'email')),

    -- A local hour is an hour of a day.
    ADD CONSTRAINT app_user_delivery_hour_check CHECK (
        delivery_hour_local IS NULL
        OR (delivery_hour_local >= 0 AND delivery_hour_local <= 23));
