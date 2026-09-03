-- A link that shows somebody a number they do not otherwise have.
--
-- The token is stored as a DIGEST and never in the clear, the same rule
-- auth_token, oauth_refresh_token and confirm_token already keep. A database
-- dump, a log line or a backup then carries nothing that opens anything — and
-- the link cannot be shown twice, because there is nothing to show it from.
--
-- Two independent ways to stop it: an expiry the server caps, and a revocation
-- somebody can perform. Neither substitutes for the other. An expiry alone
-- means a link sent to the wrong address stays open until it lapses; a
-- revocation alone means a link nobody remembers stays open forever.
SET LOCAL lock_timeout = '5s';

CREATE TABLE analytics_share (
    id uuid DEFAULT uuidv7() NOT NULL,

    -- What the recipient sees when they open it.
    --
    -- `live` re-runs the reading under the RECIPIENT's grants, so it answers
    -- what they may see today. `snapshot` serves a frozen state, filtered by
    -- the recipient's row scope. The kind is stored because the two make
    -- different promises and a reader must be told which they were handed.
    kind text NOT NULL,
    -- Which reading, and whose.
    target text NOT NULL,
    scope_kind text NOT NULL,
    scope_id uuid,
    -- The frozen state a snapshot share serves. NULL for a live share, which
    -- has none.
    snapshot_id uuid,

    -- The token's digest. NOT the token: the raw value is returned once at
    -- creation and never again.
    token_hash text NOT NULL,

    -- Who issued it. Opening the share re-checks that this person still holds
    -- forecast:read, so a departed employee's link stops serving rather than
    -- outliving their seat.
    created_by uuid NOT NULL,

    -- Both stops, and both are real. expires_at is capped server-side;
    -- revoked_at is somebody deciding sooner.
    expires_at timestamptz NOT NULL,
    revoked_at timestamptz,

    version bigint DEFAULT 1 NOT NULL,
    captured_by text NOT NULL,
    created_at timestamptz DEFAULT now() NOT NULL,
    updated_at timestamptz DEFAULT now() NOT NULL,

    CONSTRAINT analytics_share_pkey PRIMARY KEY (id),
    CONSTRAINT analytics_share_kind_check CHECK (kind IN ('live', 'snapshot')),
    CONSTRAINT analytics_share_scope_kind_check
        CHECK (scope_kind IN ('workspace', 'team', 'owner')),
    CONSTRAINT analytics_share_scope_id_matches_kind CHECK (
        (scope_kind = 'workspace') = (scope_id IS NULL)),
    -- A snapshot share names the state it serves and a live share does not.
    -- Without this a snapshot share with no snapshot would fall back to
    -- serving live data — which is a different promise from the one its kind
    -- makes to the reader.
    CONSTRAINT analytics_share_snapshot_matches_kind CHECK (
        (kind = 'snapshot') = (snapshot_id IS NOT NULL)),
    -- An expiry in the past is not a share, and a share with none is one
    -- nobody will ever remember to close.
    CONSTRAINT analytics_share_expires_after_creation CHECK (expires_at > created_at)
);

-- The lookup a request makes: one row per token, and a digest that collides is
-- a token that opens somebody else's view.
ALTER TABLE analytics_share
    ADD CONSTRAINT uq_analytics_share_token UNIQUE (token_hash);

ALTER TABLE analytics_share
    ADD CONSTRAINT analytics_share_created_by_fkey FOREIGN KEY (created_by)
    REFERENCES app_user(id) ON DELETE CASCADE;

-- A departed employee's shares go with them. CASCADE rather than RESTRICT:
-- the alternative is a deactivation that fails because somebody once shared a
-- forecast, and the link would keep serving until an administrator found it.
ALTER TABLE analytics_share
    ADD CONSTRAINT analytics_share_snapshot_fkey FOREIGN KEY (snapshot_id)
    REFERENCES forecast_snapshot(id) ON DELETE CASCADE;

CREATE INDEX idx_analytics_share_creator
    ON analytics_share (created_by, created_at DESC);
