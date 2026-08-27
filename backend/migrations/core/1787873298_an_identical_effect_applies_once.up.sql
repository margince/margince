-- An identical effect applies once. The engine fires a handler once per
-- enabled automation instance of its type, and the run claim is scoped per
-- instance (workflow_run's idempotency_key carries the automation id) — so
-- two enabled instances of one starter with identical params each plan and
-- each apply the same create against the same event, and the record they
-- mint lands twice. The invariant that was missing is effect-level: one
-- (handler, trigger event, effect fingerprint) creates once, whoever's
-- firing gets there first. This table is that claim; the engine's create
-- executor inserts here first (ON CONFLICT DO NOTHING) and a lost claim
-- skips the create while the instance's run row says it was deduplicated.
--
-- No workspace column, like workflow_run beside it: the engine is wired for
-- one installation and the bus carries no tenant.
CREATE TABLE automation_effect_claim (
    id uuid DEFAULT uuidv7() NOT NULL,
    handler text NOT NULL,
    occurrence_key text NOT NULL,
    effect_fingerprint text NOT NULL,
    created_at timestamptz DEFAULT now() NOT NULL,
    CONSTRAINT automation_effect_claim_pkey PRIMARY KEY (id),
    CONSTRAINT automation_effect_claim_unique UNIQUE (handler, occurrence_key, effect_fingerprint)
);

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE automation_effect_claim TO margince_app;
