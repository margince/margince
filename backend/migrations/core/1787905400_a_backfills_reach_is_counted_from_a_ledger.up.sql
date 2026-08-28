-- What a backfill created, one row per creation, so the count is a COUNT.
--
-- capture_backfill.people_created and organizations_created were accumulated in
-- place: each creation issued its own `SET col = col + 1` in a transaction of
-- its own, after the row it was counting had already committed elsewhere. That
-- addition can fail, and nothing replays it — capture is idempotent, so no retry
-- ever re-offers that message to the resolver. The columns were therefore a
-- FLOOR on what the run created, never an overcount, and nothing downstream
-- said so: they drive a progress display and divide into the cost estimator's
-- ratios, where a floor understates cost.
--
-- A ledger row is keyed on WHAT WAS CREATED, so writing it twice is writing it
-- once. That is what makes the write retryable, and a retryable write is the
-- difference between "a fault loses one count for every creation inside it" and
-- "a fault that outlives its retries loses one".
--
-- subject is text rather than a uuid because the two kinds are not the same
-- shape: a person is a row id, and a queued organization is a DOMAIN — capture
-- creates no companies, it opens a verdict on a domain, and the domain is what
-- identifies that work. One column, two vocabularies, told apart by kind.
--
-- No foreign key to person. The ledger records that this run created a row; a
-- person merged away or erased later does not un-create it, and a cascade would
-- silently lower a finished run's reported reach.
SET LOCAL lock_timeout = '3s';

CREATE TABLE capture_backfill_creation (
    backfill_id uuid NOT NULL REFERENCES capture_backfill(id) ON DELETE CASCADE,
    kind text NOT NULL,
    subject text NOT NULL,
    created_at timestamptz DEFAULT now() NOT NULL,
    CONSTRAINT capture_backfill_creation_pkey PRIMARY KEY (backfill_id, kind, subject),
    CONSTRAINT capture_backfill_creation_kind CHECK (kind IN ('person', 'organization_queued'))
);
