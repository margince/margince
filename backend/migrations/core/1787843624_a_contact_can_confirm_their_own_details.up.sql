-- The confirm-your-details flow: the link a contact is emailed, and what they
-- send back through it.
--
-- confirm_token is deliberately not a widened preference_token. That one is
-- plaintext, carries a 180-day ceiling and is reusable by design, because it
-- backs a List-Unsubscribe URL that must keep working for as long as mail can
-- reach an inbox. This one DISPLAYS the record and can complete a marketing
-- consent, so it is hashed at rest like consent_doi_token, short-lived, and
-- spent on first submit. Reusability is exactly what would break the consent
-- claim: a replayable link proves a mailbox was reached once, not that this
-- person chose this now.
--
-- The delivery columns are what make a consent granted here demonstrable. The
-- click stands in for a double-opt-in round trip only because the link reached
-- the subject's own mailbox, so the address it went to and the moment it was
-- sent are evidence, not telemetry.
SET LOCAL lock_timeout = '3s';

CREATE TABLE confirm_token (
    id uuid DEFAULT uuidv7() NOT NULL,
    person_id uuid NOT NULL,
    token_hash text NOT NULL,
    delivered_to text NOT NULL,
    issued_at timestamptz DEFAULT now() NOT NULL,
    expires_at timestamptz NOT NULL,
    opened_at timestamptz,
    consumed_at timestamptz,
    CONSTRAINT confirm_token_pkey PRIMARY KEY (id),
    CONSTRAINT confirm_token_hash_key UNIQUE (token_hash),
    CONSTRAINT confirm_token_person_fkey FOREIGN KEY (person_id)
        REFERENCES person (id) ON DELETE CASCADE
);

CREATE INDEX ix_confirm_token_person ON confirm_token (person_id);

-- What the subject sent back, held for a rep to accept rather than applied.
-- The subject holds a bearer token and no principal, and sits outside every
-- row-scope probe, so a leaked link must never rewrite CRM data. A correction
-- is a PROPOSAL; accepting it is a human act with its own audit row.
--
-- kind separates the two things the page can send: a corrected field value, and
-- a request to be removed. A removal carries no field and no value, which is
-- why neither column is NOT NULL.
CREATE TABLE person_confirm_submission (
    id uuid DEFAULT uuidv7() NOT NULL,
    person_id uuid NOT NULL,
    token_id uuid NOT NULL,
    kind text NOT NULL,
    field text,
    proposed_value text,
    submitted_at timestamptz DEFAULT now() NOT NULL,
    resolved_at timestamptz,
    resolved_by text,
    resolution text,
    CONSTRAINT person_confirm_submission_pkey PRIMARY KEY (id),
    CONSTRAINT person_confirm_submission_person_fkey FOREIGN KEY (person_id)
        REFERENCES person (id) ON DELETE CASCADE,
    CONSTRAINT person_confirm_submission_token_fkey FOREIGN KEY (token_id)
        REFERENCES confirm_token (id) ON DELETE CASCADE,
    CONSTRAINT person_confirm_submission_kind_check
        CHECK (kind IN ('correction', 'erasure_request')),
    CONSTRAINT person_confirm_submission_resolution_check
        CHECK (resolution IS NULL OR resolution IN ('accepted', 'rejected')),
    -- A correction names the field it corrects; a removal request names none.
    CONSTRAINT person_confirm_submission_field_matches_kind
        CHECK ((kind = 'correction' AND field IS NOT NULL)
            OR (kind = 'erasure_request' AND field IS NULL)),
    -- Resolved means all three resolution columns are set, or none are. A row
    -- carrying a verdict with nobody answerable for it is not a decision.
    CONSTRAINT person_confirm_submission_resolved_together
        CHECK (num_nonnulls(resolved_at, resolved_by, resolution) IN (0, 3))
);

CREATE INDEX ix_person_confirm_submission_person ON person_confirm_submission (person_id);
-- The rep's queue: what is still waiting, oldest first.
CREATE INDEX ix_person_confirm_submission_open ON person_confirm_submission (submitted_at)
    WHERE resolved_at IS NULL;

COMMENT ON TABLE confirm_token IS
    'Single-use, hashed, short-lived capability for the confirm-your-details page. Delivered to one address, which is what makes a consent granted through it demonstrable.';
COMMENT ON COLUMN confirm_token.delivered_to IS
    'The address this link was mailed to, recorded because a grant made through it rests on the link having reached that mailbox.';
COMMENT ON COLUMN confirm_token.opened_at IS
    'When the page was first opened with this token, so the ask-to-click chain is readable from the row.';
COMMENT ON TABLE person_confirm_submission IS
    'What a data subject sent back through their confirm link, held for a human to accept. The subject holds a bearer token and no principal, so a correction is a proposal and never a write.';

GRANT SELECT, INSERT, DELETE, UPDATE ON TABLE confirm_token TO margince_app;
GRANT SELECT, INSERT, DELETE, UPDATE ON TABLE person_confirm_submission TO margince_app;
