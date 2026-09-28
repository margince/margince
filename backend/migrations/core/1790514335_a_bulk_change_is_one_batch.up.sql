-- A bulk change over a selection of contacts, companies or deals.
--
-- Each record the change touches keeps its own audit row and its own event,
-- exactly as a single-record change would write them. What ties them together
-- is batch_id: every audit row one bulk change writes carries the id of that
-- change, and bulk_operation records who asked for it and what they asked.
SET LOCAL lock_timeout = '5s';

-- Nullable with no default, so adding it rewrites nothing. The index is partial
-- because almost every audit row belongs to no batch.
ALTER TABLE audit_log ADD COLUMN batch_id uuid;
CREATE INDEX idx_audit_log_batch ON audit_log (batch_id) WHERE batch_id IS NOT NULL;

-- One row per executed bulk change. The audit rows carrying its id are the
-- per-record detail; this row is the request itself.
CREATE TABLE bulk_operation (
    id uuid DEFAULT uuidv7() NOT NULL,
    record_type text NOT NULL,
    verb text NOT NULL,
    -- The verb's parameters as the caller sent them: the new owner for a
    -- reassignment, nothing for an archive.
    params jsonb NOT NULL DEFAULT '{}'::jsonb,
    -- The typed principal id that asked, spelled as audit_log.actor_id is.
    requested_by text NOT NULL,
    passport_id uuid,
    changed_count integer NOT NULL,
    skipped_count integer NOT NULL,
    created_at timestamptz DEFAULT now() NOT NULL,

    CONSTRAINT bulk_operation_pkey PRIMARY KEY (id),
    CONSTRAINT bulk_operation_record_type_check CHECK (record_type IN ('contact', 'company', 'deal')),
    CONSTRAINT bulk_operation_verb_check CHECK (verb IN ('reassign_owner', 'archive')),
    CONSTRAINT bulk_operation_counts_check CHECK (changed_count >= 0 AND skipped_count >= 0)
);

CREATE INDEX idx_bulk_operation_requested_by ON bulk_operation (requested_by, created_at);
CREATE INDEX idx_bulk_operation_created ON bulk_operation (created_at);

-- The user's confirmation of one previewed bulk change. A preview that would
-- change anything mints a token; the table keeps only its hash, and the
-- execution that presents it spends the row in its own transaction, so one
-- confirmation opens exactly one execution.
CREATE TABLE bulk_confirmation (
    token_hash bytea NOT NULL,
    -- Who may spend it: the typed principal id that previewed.
    requested_by text NOT NULL,
    -- The hash of what was previewed: record type, verb, parameters and every
    -- item with its version. An execution of anything else does not match.
    binding bytea NOT NULL,
    -- The records the preview said would change. The execution changes no
    -- record outside them, whatever has moved since.
    affected uuid[] NOT NULL,
    expires_at timestamptz NOT NULL,
    consumed_at timestamptz,
    created_at timestamptz DEFAULT now() NOT NULL,

    CONSTRAINT bulk_confirmation_pkey PRIMARY KEY (token_hash),
    CONSTRAINT bulk_confirmation_expiry_after_creation CHECK (expires_at > created_at)
);

CREATE INDEX idx_bulk_confirmation_expires ON bulk_confirmation (expires_at);
