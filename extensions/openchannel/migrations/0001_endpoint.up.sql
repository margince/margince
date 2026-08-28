-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion
--
-- One row per opened endpoint: which declared anonymous edge it is, whose
-- consent stands behind it, where its replies go, and what the screen renders
-- about traffic. It is applied by the pre-merge gate as a restricted
-- ext_openchannel role and at runtime by margince_owner; what bounds it in
-- production is the grant surface that gate polices and the ext schema this
-- unit owns.
--
-- NEITHER THE SIGNING SECRET NOR ANY OTHER CREDENTIAL IS HERE. The secret an
-- inbound request is verified against lives in this unit's user-scoped secret
-- namespace, sealed by the installation's custodian. This table records only
-- THAT an endpoint was opened and who owns it — which is also what makes every
-- column of it safe for a screen to render.

CREATE TABLE ext.ext_openchannel_endpoint (
    id              uuid        NOT NULL DEFAULT gen_random_uuid() PRIMARY KEY,

    -- WHICH declared inbound edge this row owns. The unit declares its edges
    -- as literals, so a slug is not a value a member invents: opening an
    -- endpoint CLAIMS one of the declared slugs, and the anonymous handler
    -- resolves the slug it was called on back to this row to find the owner
    -- and the secret. UNIQUE because two rows claiming one slug would be two
    -- answers to "whose endpoint is this", and the handler would take whichever
    -- the planner returned.
    slug            text        NOT NULL CHECK (length(slug) > 0),

    -- WHOSE endpoint this is: the member whose sealed secret verifies every
    -- request that arrives on it, and whose authority the drain will act
    -- under. Stamped by the handler from the invocation's Caller and never
    -- from the request body — a user id a client supplies is a user id a
    -- client forges, and here it would forge the consent that authority is
    -- read from.
    --
    -- NO FOREIGN KEY to the core user table: the role this file is applied as
    -- holds REFERENCES on workspace and nothing else on public, so there is no
    -- constraint to declare. The cost is real and worth stating — nothing
    -- deletes this row when the account is deleted, so a reader must treat the
    -- id as one that may no longer resolve, and every path that acts for the
    -- owner has to survive an owner who is gone.
    user_id         uuid        NOT NULL,

    -- Where this connector sends OUTWARD, registered separately from opening
    -- the endpoint and NULL until it is: an endpoint that only receives is a
    -- complete, useful thing, and a placeholder URL would be one the outbound
    -- half would eventually try to dial.
    url             text        CHECK (url IS NULL OR length(url) > 0),

    -- Whether the edge is currently accepting. A disabled endpoint keeps its
    -- row, its owner and its sealed secret — disabling is a pause a member can
    -- undo, and the alternative (delete the row) would destroy the secret every
    -- registered sender already holds.
    enabled         boolean     NOT NULL DEFAULT true,

    -- What the screen renders about traffic. A count and a last-seen time in
    -- each direction, and no more: the queue table below holds the individual
    -- requests, so anything finer belongs to a query over it rather than to a
    -- counter that can only ever disagree with one.
    inbound_received bigint     NOT NULL DEFAULT 0 CHECK (inbound_received >= 0),
    outbound_sent    bigint     NOT NULL DEFAULT 0 CHECK (outbound_sent >= 0),
    last_inbound_at  timestamptz,
    last_outbound_at timestamptz,

    -- Optimistic concurrency for the row, incremented by every governed write.
    -- Two writers here are ordinary: a member disabling the endpoint while
    -- another tab registers a URL.
    --
    -- The anonymous inbound path deliberately does NOT touch it. It moves the
    -- two traffic columns and nothing a member decided, so counting it as a
    -- version would make every arriving request a lost update for whoever had
    -- the screen open.
    version         integer     NOT NULL DEFAULT 1 CHECK (version > 0),
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT ext_openchannel_endpoint_one_per_slug UNIQUE (slug),
    -- One endpoint per member, so "this member's endpoint" is a question with
    -- one answer everywhere the governed surface asks it.
    CONSTRAINT ext_openchannel_endpoint_one_per_member UNIQUE (user_id)
);

-- The app role runs the unit's handlers. TRUNCATE is deliberately absent: no
-- operation here empties the table, and a privilege nothing reaches for is one
-- more thing a compromised unit could.
GRANT SELECT, INSERT, UPDATE, DELETE ON ext.ext_openchannel_endpoint TO margince_app;
