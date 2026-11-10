-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- One person's sales goals for one calendar year, agreed with their manager.
-- The monthly sales_target rows stay the reported goals. The plan holds the
-- shared draft and the history of what was published, so twelve months are
-- set, published and withdrawn together. Revisions are never rewritten; the
-- header points at the current draft and publication.
SET LOCAL lock_timeout = '3s';

CREATE TABLE sales_goal_plan (
    id uuid NOT NULL DEFAULT uuidv7(),
    owner_id uuid NOT NULL,
    year integer NOT NULL,
    currency currency_code NOT NULL,
    version bigint NOT NULL,
    draft_revision bigint,
    published_revision bigint,
    withdrawn_revision bigint,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,

    CONSTRAINT sales_goal_plan_pkey PRIMARY KEY (id),
    CONSTRAINT sales_goal_plan_owner_id_fkey FOREIGN KEY (owner_id) REFERENCES app_user(id),
    CONSTRAINT sales_goal_plan_owner_year UNIQUE (owner_id, year),
    CONSTRAINT sales_goal_plan_year_check CHECK (year BETWEEN 2000 AND 2200),
    CONSTRAINT sales_goal_plan_version_js_safe CHECK (version >= 1 AND version <= '9007199254740991'::bigint),
    CONSTRAINT sales_goal_plan_draft_revision_js_safe CHECK (draft_revision >= 1 AND draft_revision <= '9007199254740991'::bigint),
    CONSTRAINT sales_goal_plan_published_revision_js_safe CHECK (published_revision >= 1 AND published_revision <= '9007199254740991'::bigint),
    CONSTRAINT sales_goal_plan_withdrawn_revision_js_safe CHECK (withdrawn_revision >= 1 AND withdrawn_revision <= '9007199254740991'::bigint),
    -- A plan is published or withdrawn, never both at once.
    CONSTRAINT sales_goal_plan_one_publication_state CHECK (published_revision IS NULL OR withdrawn_revision IS NULL)
);

-- months holds twelve entries, January first, each minor units or null for
-- Not set. targets is set on a publication: the twelve target revisions it
-- wrote, so a later change by another route is visible.
CREATE TABLE sales_goal_plan_revision (
    plan_id uuid NOT NULL,
    revision bigint NOT NULL,
    kind text NOT NULL,
    months jsonb NOT NULL,
    declared_total bigint,
    origin text NOT NULL,
    reason text NOT NULL,
    targets jsonb,
    created_by uuid NOT NULL,
    created_at timestamptz NOT NULL,

    CONSTRAINT sales_goal_plan_revision_pkey PRIMARY KEY (plan_id, revision),
    CONSTRAINT sales_goal_plan_revision_plan_id_fkey FOREIGN KEY (plan_id) REFERENCES sales_goal_plan(id),
    CONSTRAINT sales_goal_plan_revision_created_by_fkey FOREIGN KEY (created_by) REFERENCES app_user(id),
    CONSTRAINT sales_goal_plan_revision_kind_check CHECK (kind IN ('draft', 'published', 'discarded', 'withdrawn')),
    CONSTRAINT sales_goal_plan_revision_origin_check CHECK (origin IN ('manual', 'ai', 'adopted')),
    CONSTRAINT sales_goal_plan_revision_months_shape CHECK (jsonb_typeof(months) = 'array' AND jsonb_array_length(months) = 12),
    CONSTRAINT sales_goal_plan_revision_targets_shape CHECK ((kind = 'published') = (targets IS NOT NULL)),
    CONSTRAINT sales_goal_plan_revision_reason_bounded CHECK (length(btrim(reason)) BETWEEN 1 AND 1000),
    CONSTRAINT sales_goal_plan_revision_revision_js_safe CHECK (revision >= 1 AND revision <= '9007199254740991'::bigint),
    CONSTRAINT sales_goal_plan_revision_declared_total_js_safe CHECK (declared_total >= 0 AND declared_total <= '9007199254740991'::bigint)
);

CREATE INDEX idx_sales_goal_plan_revision_created_by ON sales_goal_plan_revision (created_by);
