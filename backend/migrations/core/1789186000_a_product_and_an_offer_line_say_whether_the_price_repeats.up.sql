-- A product and an offer line say whether the price repeats, and how often.
--
-- Three states, and the third one is the point. A row may say the price is
-- one_time, or recurring on a stated cadence, or say NOTHING — which is what
-- every row written before this migration says, and it must keep saying it.
-- Defaulting those to one_time would assert a classification nobody made, and
-- the assertion would then be indistinguishable from an answer.
--
-- So billing_model is nullable with no default, and the shape CHECK admits
-- exactly the three combinations rather than constraining the columns
-- separately. Written null-safe: a bare `billing_model = 'recurring'` is NULL
-- for an unclassified row, and a CHECK that evaluates NULL passes, which is how
-- a constraint comes to admit the rows it was written to refuse.

SET LOCAL lock_timeout = '3s';

ALTER TABLE product ADD COLUMN billing_model text;
ALTER TABLE product ADD COLUMN billing_interval_months integer;

ALTER TABLE product ADD CONSTRAINT product_billing_model_check
    CHECK (billing_model IS NULL OR billing_model IN ('one_time', 'recurring'));

-- The cadence belongs to a recurring price and to nothing else. An unclassified
-- row carries none because it makes no claim; a one_time row carries none
-- because there is nothing to repeat.
-- IS NOT DISTINCT FROM, not =. A bare `billing_model = 'one_time'` is NULL for
-- an unclassified row, every arm of the disjunction then evaluates to NULL, and
-- a CHECK that evaluates NULL PASSES. Written with = this constraint admitted a
-- row carrying a cadence and no model — verified against a live schema before
-- it was rewritten, which is the only way that class of mistake is visible.
ALTER TABLE product ADD CONSTRAINT product_billing_shape
    CHECK (
        (billing_model IS NULL AND billing_interval_months IS NULL)
        OR (billing_model IS NOT DISTINCT FROM 'one_time' AND billing_interval_months IS NULL)
        OR (billing_model IS NOT DISTINCT FROM 'recurring' AND billing_interval_months IS NOT NULL)
    );

ALTER TABLE product ADD CONSTRAINT product_billing_interval_check
    CHECK (billing_interval_months IS NULL OR billing_interval_months IN (1, 3, 6, 12));

COMMENT ON COLUMN product.billing_model IS
    'one_time, recurring, or null for a product nobody has classified. Null is not one_time: it is the absence of an answer, and every row written before this column existed carries it.';

-- The same pair, SNAPSHOTTED onto the line, plus how many periods the buyer
-- committed to. Snapshotted rather than joined because an offer that has been
-- sent is a document: reclassifying the product afterwards must not change what
-- the paper said.
ALTER TABLE offer_line_item ADD COLUMN billing_model text;
ALTER TABLE offer_line_item ADD COLUMN billing_interval_months integer;
ALTER TABLE offer_line_item ADD COLUMN interval_count integer;

ALTER TABLE offer_line_item ADD CONSTRAINT oli_billing_model_check
    CHECK (billing_model IS NULL OR billing_model IN ('one_time', 'recurring'));

ALTER TABLE offer_line_item ADD CONSTRAINT oli_billing_interval_check
    CHECK (billing_interval_months IS NULL OR billing_interval_months IN (1, 3, 6, 12));

-- interval_count is the committed number of periods. It belongs to a recurring
-- line alone, and it is optional WHILE DRAFTING: a line can be classified
-- before anybody has settled how long the commitment runs. Send refuses a
-- recurring line that still has none, which is a lifecycle rule rather than a
-- shape one and lives in Go where it can say so.
-- Null-safe for the reason product_billing_shape states above.
ALTER TABLE offer_line_item ADD CONSTRAINT oli_billing_shape
    CHECK (
        (billing_model IS NULL AND billing_interval_months IS NULL AND interval_count IS NULL)
        OR (billing_model IS NOT DISTINCT FROM 'one_time'
            AND billing_interval_months IS NULL AND interval_count IS NULL)
        OR (billing_model IS NOT DISTINCT FROM 'recurring' AND billing_interval_months IS NOT NULL)
    );

ALTER TABLE offer_line_item ADD CONSTRAINT oli_interval_count_check
    CHECK (interval_count IS NULL OR interval_count > 0);

COMMENT ON COLUMN offer_line_item.interval_count IS
    'How many billing periods the buyer commits to. Null on a recurring line means nobody has settled the term yet, which send refuses.';

-- Where a deal's recurring figure came from.
--
-- Nullable, and pointing at an offer on the SAME deal — a provenance naming
-- another deal's accepted offer would explain a figure with a document that
-- prices something else. Enforced by a composite FK to (deal_id, id), which
-- needs that pair to be unique first.
ALTER TABLE offer ADD CONSTRAINT offer_deal_id_id_key UNIQUE (deal_id, id);

ALTER TABLE deal ADD COLUMN arr_source_offer_id uuid;

ALTER TABLE deal ADD CONSTRAINT deal_arr_source_same_deal
    FOREIGN KEY (id, arr_source_offer_id) REFERENCES offer (deal_id, id) ON DELETE SET NULL;

COMMENT ON COLUMN deal.arr_source_offer_id IS
    'The accepted offer this deal''s expected_arr_minor came from, or null where a human set the figure. A deal whose ARR has a source refuses manual edits to it.';
