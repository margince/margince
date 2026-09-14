SET LOCAL lock_timeout = '3s';

ALTER TABLE relationship ADD COLUMN employment_status text CHECK (employment_status IN ('current', 'former', 'unknown'));
ALTER TABLE relationship ADD COLUMN started_precision text CHECK (started_precision IN ('day', 'month'));
ALTER TABLE relationship ADD COLUMN ended_precision text CHECK (ended_precision IN ('day', 'month'));
ALTER TABLE relationship ADD CONSTRAINT rel_employment_status_kind CHECK (employment_status IS NULL OR kind = 'employment');
ALTER TABLE relationship ADD CONSTRAINT rel_employment_primary_status CHECK (NOT is_current_primary OR coalesce(employment_status, 'current') = 'current');
DROP INDEX uq_rel_employment;
CREATE UNIQUE INDEX uq_rel_employment ON relationship (contact_id, company_id)
 WHERE kind = 'employment' AND ended_at IS NULL AND archived_at IS NULL AND coalesce(employment_status, 'current') = 'current';

CREATE TABLE provider_employment_resolution (
 id uuid PRIMARY KEY DEFAULT uuidv7(),
 contact_id uuid NOT NULL REFERENCES contact(id) ON DELETE CASCADE,
 claim_id uuid NOT NULL REFERENCES contact_provider_claim(id) ON DELETE CASCADE,
 episode_key text NOT NULL,
 state text NOT NULL CHECK (state IN ('linked', 'needs_match', 'needs_review', 'dismissed')),
 company_id uuid REFERENCES company(id) ON DELETE SET NULL,
 relationship_id uuid REFERENCES relationship(id) ON DELETE SET NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE (claim_id, episode_key)
);
CREATE INDEX provider_employment_contact ON provider_employment_resolution(contact_id, episode_key);
GRANT SELECT, INSERT, UPDATE, DELETE ON provider_employment_resolution TO margince_app;
ALTER TABLE contact_provider_claim ADD COLUMN employment_processed_at timestamptz;
CREATE INDEX provider_employment_pending ON contact_provider_claim(contact_id) WHERE employment_processed_at IS NULL AND claim_key IN ('current_employment','job_history');
UPDATE contact_provider_claim SET employment_processed_at=now() WHERE claim_key IN ('current_employment','job_history');
