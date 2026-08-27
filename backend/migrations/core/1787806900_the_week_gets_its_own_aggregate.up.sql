-- The weekly review, in its own aggregate.
--
-- NOT ON brief_run, and not for tidiness. A weekly row living there would
-- become "the latest morning brief" to every reader that orders by
-- generated_at, and briefLastView reads exactly that to decide the next
-- morning's overnight window — so a Friday weekly would silently reset what
-- Saturday's brief counts as "changed overnight".
--
-- NOT ON brief_item either. brief_item.deal_id carries ON DELETE CASCADE from
-- deal, so a deleted deal removes rows from every past brief. That is right for
-- a queue, which is about deals that exist; it is wrong for a retrospective,
-- which is a record of what a week WAS. A past week that quietly loses a line
-- because somebody cleaned up a deal is a record nobody can trust.
--
-- So the content is FROZEN, the way audit_log freezes its before/after images
-- and an issued offer freezes buyer_snapshot: the subject's id is stored
-- without a foreign key, beside the label it carried at the time.
SET LOCAL lock_timeout = '3s';

CREATE TABLE weekly_review (
    id uuid DEFAULT uuidv7() NOT NULL,
    user_id uuid NOT NULL,
    -- The Monday of the week under review, as a calendar date in the
    -- installation's reporting zone — the same zone brief_run.local_day is
    -- stamped in. Stored rather than derived because the zone is an
    -- operator-mutable setting, and re-deriving it later would re-label weeks
    -- that already exist.
    local_week_start date NOT NULL,
    generated_at timestamptz DEFAULT now() NOT NULL,
    -- The instant the week was measured to. Every count below is as-of this,
    -- so a reader knows what the numbers were true of.
    as_of timestamptz NOT NULL,

    -- Promised vs delivered. Tasks are countable honestly: activity_done_at
    -- guarantees done_at is set when is_done, so "kept this week" is a real
    -- window rather than an inference.
    tasks_due int DEFAULT 0 NOT NULL,
    tasks_done int DEFAULT 0 NOT NULL,
    tasks_carried_over int DEFAULT 0 NOT NULL,

    -- Deals that moved, and how they ended.
    deals_moved int DEFAULT 0 NOT NULL,
    deals_won int DEFAULT 0 NOT NULL,
    deals_lost int DEFAULT 0 NOT NULL,

    -- What the rep did with what Margince proposed. HUMAN decisions only —
    -- the expiry sweep also stamps approval.decided_at, with decided_by NULL,
    -- and counting those as acceptances would credit the rep with decisions
    -- nobody made.
    proposals_accepted int DEFAULT 0 NOT NULL,
    proposals_rejected int DEFAULT 0 NOT NULL,
    -- And with the morning queue itself.
    brief_items_acted int DEFAULT 0 NOT NULL,
    brief_items_dismissed int DEFAULT 0 NOT NULL,

    CONSTRAINT weekly_review_pkey PRIMARY KEY (id),
    -- Every count is a tally, and a negative one is a broken writer rather
    -- than a small week.
    CONSTRAINT weekly_review_counts_are_tallies CHECK (
        tasks_due >= 0 AND tasks_done >= 0 AND tasks_carried_over >= 0
        AND deals_moved >= 0 AND deals_won >= 0 AND deals_lost >= 0
        AND proposals_accepted >= 0 AND proposals_rejected >= 0
        AND brief_items_acted >= 0 AND brief_items_dismissed >= 0)
);

-- One review per rep per week, and the constraint is the correctness rather
-- than the read-then-insert that precedes it: the dispatcher ticks more than
-- once inside a week on purpose, so that a worker which was down still
-- backfills, and this is what collapses those ticks into one review.
ALTER TABLE weekly_review
    ADD CONSTRAINT uq_weekly_review_user_week UNIQUE (user_id, local_week_start);

-- A departed rep's own retrospectives go with them, exactly as their briefs do
-- (brief_run_user_fkey). The week's FACTS live on the records themselves; what
-- this table holds is one person's reading of them.
ALTER TABLE weekly_review
    ADD CONSTRAINT weekly_review_user_fkey FOREIGN KEY (user_id)
    REFERENCES app_user(id) ON DELETE CASCADE;

-- The archive read: this rep's weeks, newest first.
CREATE INDEX idx_weekly_review_user ON weekly_review (user_id, local_week_start DESC);

-- One line per deal the week is about, frozen.
--
-- NO FOREIGN KEY ON deal_id, and that is the point of the table. The label is
-- stored beside the id so a deal deleted next month leaves last month's
-- retrospective still saying what it said — the same reason audit_log carries
-- entity_id unconstrained and an issued offer carries buyer_snapshot.
CREATE TABLE weekly_review_deal (
    id uuid DEFAULT uuidv7() NOT NULL,
    weekly_review_id uuid NOT NULL,
    deal_id uuid NOT NULL,
    -- What the deal was called that week. A rename later does not rewrite
    -- history, and a deletion does not erase it.
    deal_label text NOT NULL,
    -- What happened to it: it moved stage, it was won, or it was lost.
    outcome text NOT NULL,
    -- Where it went, as words rather than stage ids: a renamed or deleted
    -- stage must not make an old review unreadable.
    to_stage_label text,
    amount_minor_at_close bigint,
    currency_at_close text,
    occurred_at timestamptz NOT NULL,

    CONSTRAINT weekly_review_deal_pkey PRIMARY KEY (id),
    CONSTRAINT weekly_review_deal_outcome_check CHECK (outcome IN ('moved', 'won', 'lost')),
    -- A label nobody can read is not a frozen fact.
    CONSTRAINT weekly_review_deal_label_present CHECK (btrim(deal_label) <> ''),
    -- Money is a pair or it is absent: an amount with no currency is a number
    -- nobody can read, and this tree refuses to print one.
    CONSTRAINT weekly_review_deal_money_whole CHECK (
        (amount_minor_at_close IS NULL) = (currency_at_close IS NULL))
);

ALTER TABLE weekly_review_deal
    ADD CONSTRAINT weekly_review_deal_review_fkey FOREIGN KEY (weekly_review_id)
    REFERENCES weekly_review(id) ON DELETE CASCADE;

CREATE INDEX idx_weekly_review_deal_review ON weekly_review_deal (weekly_review_id, occurred_at DESC);
