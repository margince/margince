-- A reader sets the deal card's move aside: the deal page's "Not now".
--
-- Per-reader view state, like contact_moment_dismissal and
-- suggestion_dismissal: it feeds no formula and no other reader's page, so it
-- carries no audit row. The fingerprint is a digest of the move the card
-- named; a different move does not match it, so the set-aside lifts by itself.
-- One row per reader and deal, because the card names one move.
SET LOCAL lock_timeout = '5s';

CREATE TABLE deal_move_dismissal (
    user_id uuid NOT NULL,
    deal_id uuid NOT NULL,
    fingerprint text NOT NULL,
    dismissed_at timestamptz DEFAULT now() NOT NULL,
    CONSTRAINT deal_move_dismissal_pkey PRIMARY KEY (user_id, deal_id),
    CONSTRAINT deal_move_dismissal_fingerprint_shape CHECK (fingerprint ~ '^[0-9a-f]{64}$'),
    CONSTRAINT deal_move_dismissal_user_fkey FOREIGN KEY (user_id)
        REFERENCES app_user(id) ON DELETE CASCADE,
    CONSTRAINT deal_move_dismissal_deal_fkey FOREIGN KEY (deal_id)
        REFERENCES deal(id) ON DELETE CASCADE
);

CREATE INDEX deal_move_dismissal_deal_ix ON deal_move_dismissal (deal_id);

COMMENT ON TABLE deal_move_dismissal IS 'Per-reader view state: the deal card move this reader set aside, held against the move fingerprint so a different move shows again.';
