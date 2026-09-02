-- The grant goes with the object it names: leaving it behind would have every
-- role document claim authority over a table that no longer exists — and
-- policy.Parse REFUSES a document naming an object outside the closed set, so
-- the leftover key would fail every login rather than merely being unused.
UPDATE role SET permissions = permissions #- '{objects,weekly_plan}'
    WHERE permissions ? 'objects' AND (permissions -> 'objects') ? 'weekly_plan';
