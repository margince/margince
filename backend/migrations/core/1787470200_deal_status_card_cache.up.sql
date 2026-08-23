-- The deal page's one written card, cached on the facts it was written from.
--
-- Keyed per READER, like person_brief beside it and for the same reason: the
-- card is assembled under the caller's row scope, so a card written for one
-- person is not a card another may read. Sharing a row would either leak a
-- scoped activity to a restricted colleague or serve the deal's owner the
-- thinner card that colleague gets.
--
-- The fingerprint covers the assembled input, the prompt version and the
-- reader, so a stale card is rewritten rather than served. There is no TTL
-- column: freshness is a property of the facts, not of the clock.
SET LOCAL lock_timeout = '5s';

CREATE TABLE deal_status_card (
    user_id uuid NOT NULL,
    deal_id uuid NOT NULL,
    fingerprint text NOT NULL,
    payload jsonb NOT NULL,
    generated_by text NOT NULL,
    generated_at timestamptz DEFAULT now() NOT NULL,
    CONSTRAINT deal_status_card_pkey PRIMARY KEY (user_id, deal_id),
    CONSTRAINT deal_status_card_generated_by_check CHECK (generated_by IN ('model', 'deterministic')),
    -- The payload holds sentences written from mail subjects, body excerpts and
    -- Deal Room comments. Deleting the deal or the reader has to take that text
    -- with it, and a cascade is the only thing that reaches a derived row no
    -- code path enumerates. person_brief carries the same pair for the same
    -- reason.
    CONSTRAINT deal_status_card_deal_fkey FOREIGN KEY (deal_id)
        REFERENCES deal(id) ON DELETE CASCADE,
    CONSTRAINT deal_status_card_user_fkey FOREIGN KEY (user_id)
        REFERENCES app_user(id) ON DELETE CASCADE
);

-- The primary key leads on user_id, so a delete cascading from the DEAL side
-- would scan without this.
CREATE INDEX deal_status_card_deal_ix ON deal_status_card USING btree (deal_id);

COMMENT ON TABLE deal_status_card IS 'Read-model cache for the deal status card, keyed per READER: no card crosses readers, whatever their scope.';
