-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- The bars go back to one row each, with the checks the table carried.
SET LOCAL lock_timeout = '3s';

CREATE TABLE weekly_review_movement (
    id uuid DEFAULT uuidv7() NOT NULL,
    weekly_review_id uuid NOT NULL,
    period_kind text NOT NULL,
    bar text NOT NULL,
    delta_minor bigint NOT NULL,
    CONSTRAINT weekly_review_movement_pkey PRIMARY KEY (id),
    CONSTRAINT weekly_review_movement_period_kind_check
        CHECK (period_kind IN ('week', 'month', 'quarter')),
    CONSTRAINT weekly_review_movement_bar_check
        CHECK (bar IN ('created', 'advanced', 'slipped', 'excluded', 'reinstated', 'not_checked', 'won', 'lost', 'other')),
    CONSTRAINT weekly_review_movement_delta_minor_js_safe
        CHECK (delta_minor >= '-9007199254740991'::bigint AND delta_minor <= '9007199254740991'::bigint),
    CONSTRAINT weekly_review_movement_review_fkey FOREIGN KEY (weekly_review_id)
        REFERENCES weekly_review(id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX uq_weekly_review_movement_bar
    ON weekly_review_movement (weekly_review_id, period_kind, bar);
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE weekly_review_movement TO margince_app;

INSERT INTO weekly_review_movement (weekly_review_id, period_kind, bar, delta_minor)
SELECT r.id, b.period_kind, b.bar, b.delta_minor
  FROM weekly_review r
 CROSS JOIN LATERAL jsonb_to_recordset(r.outlook_movement) AS b(period_kind text, bar text, delta_minor bigint)
 WHERE r.outlook_movement IS NOT NULL;

CREATE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."weekly_review_movement" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"bar":20000,"period_kind":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"bar":20000,"period_kind":20000}');
CREATE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."weekly_review_movement" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"bar":20000,"period_kind":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"bar":20000,"period_kind":20000}');

ALTER TABLE weekly_review DROP COLUMN outlook_movement;
