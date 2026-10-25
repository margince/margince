-- A completed privacy request keeps a proof of what was done, for three years
-- after the end of the year it was answered in, and then the proof goes.
--
-- The request row alone cannot prove an answer: its resolution is prose, and
-- who acted is only in audit_log. The proof is written in the transaction that
-- closes the request and holds no mail content and no contact data beyond the
-- requester's own address.
SET LOCAL lock_timeout = '3s';

CREATE TABLE privacy_request_proof (
    id uuid DEFAULT uuidv7() NOT NULL,
    request_id uuid NOT NULL,
    -- The receipt reference the subject was given, or the request id when the
    -- request was filed by hand and has none.
    request_number text NOT NULL,
    -- NULL when the request did not come from an address, such as a letter.
    requester_email text,
    received_at timestamptz NOT NULL,
    request_kind text NOT NULL,
    identity_check text NOT NULL,
    result text NOT NULL,
    -- "<table>.<verb>" to the rows the erasure's statements touched there.
    -- Empty for a request that erased nothing.
    erased_counts jsonb DEFAULT '{}'::jsonb NOT NULL,
    -- The erasure's arms that destroyed something outside the database, where
    -- no row count shows how much.
    erased_uncounted text[] DEFAULT '{}'::text[] NOT NULL,
    -- false on a proof written for a request answered before proofs existed,
    -- when no record shows what its erasure removed.
    erased_counts_known boolean DEFAULT true NOT NULL,
    -- Who closed the request, as audit_log names an actor. NULL only on a
    -- backfilled proof whose request was closed without an audit row.
    completed_by_type text,
    completed_by_id text,
    answered_at timestamptz NOT NULL,
    keep_until timestamptz NOT NULL,
    CONSTRAINT privacy_request_proof_pkey PRIMARY KEY (id),
    CONSTRAINT privacy_request_proof_one_per_request UNIQUE (request_id),
    CONSTRAINT privacy_request_proof_request_fkey FOREIGN KEY (request_id)
        REFERENCES data_subject_request (id) ON DELETE CASCADE,
    CONSTRAINT privacy_request_proof_kind CHECK (
        request_kind = ANY (ARRAY['access'::text, 'rectify'::text, 'erasure'::text])),
    CONSTRAINT privacy_request_proof_identity_check CHECK (
        identity_check = ANY (ARRAY['confirm_link'::text, 'address_on_file'::text,
            'id_document'::text, 'in_person'::text, 'not_verified'::text, 'unknown'::text])),
    CONSTRAINT privacy_request_proof_result CHECK (
        result = ANY (ARRAY['fulfilled'::text, 'rejected'::text])),
    CONSTRAINT privacy_request_proof_counts_object CHECK (jsonb_typeof(erased_counts) = 'object'),
    CONSTRAINT privacy_request_proof_actor_type CHECK (
        completed_by_type = ANY (ARRAY['human'::text, 'agent'::text, 'connector'::text, 'system'::text]))
);

-- The January sweep reads expired proofs in this order.
CREATE INDEX privacy_request_proof_keep_until ON privacy_request_proof (keep_until, id);

-- An open complaint or dispute about the request. While it is set, the
-- request's proof outlives its retention date.
ALTER TABLE data_subject_request
    ADD COLUMN dispute_open boolean DEFAULT false NOT NULL;
