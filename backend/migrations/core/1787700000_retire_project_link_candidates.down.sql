-- Rebuilds the retired rung's table, empty, exactly as 1787409906 created it.
--
-- The rows are NOT restored and cannot be: the up migration dropped them with
-- the table. Down puts the schema back so an older binary starts and can write
-- new candidates; it does not put the old questions back, and nothing needs it
-- to — a rebuilt empty table is what an installation that never captured
-- anything has.
--
-- The approvals those candidates rode on were never touched by the up migration,
-- so any that are still pending are still there. An older binary that reads them
-- finds its ledger row missing and resolves by proposal id against nothing, which
-- is the same state a crash between staging and recording always produced: the
-- decision still applies, only the coverage count is lost.
SET LOCAL lock_timeout = '5s';

CREATE TABLE project_link_candidate (
    id uuid DEFAULT uuidv7() NOT NULL,
    activity_id uuid NOT NULL,
    project_id uuid NOT NULL,
    method text NOT NULL,
    confidence numeric(5,4) NOT NULL,
    evidence_field text,
    evidence_start integer,
    evidence_end integer,
    proposal_id uuid,
    status text DEFAULT 'pending'::text NOT NULL,
    decided_at timestamptz,
    version bigint DEFAULT 1 NOT NULL,
    created_at timestamptz DEFAULT now() NOT NULL,
    updated_at timestamptz DEFAULT now() NOT NULL,
    CONSTRAINT project_link_candidate_pkey PRIMARY KEY (id),
    CONSTRAINT project_link_candidate_activity_id_fkey FOREIGN KEY (activity_id)
        REFERENCES activity(id) ON DELETE CASCADE,
    CONSTRAINT project_link_candidate_project_id_fkey FOREIGN KEY (project_id)
        REFERENCES project(id) ON DELETE CASCADE,
    CONSTRAINT project_link_candidate_proposal_id_fkey FOREIGN KEY (proposal_id)
        REFERENCES approval(id) ON DELETE SET NULL,
    CONSTRAINT project_link_candidate_method_check
        CHECK (method IN ('sole_live_project', 'ranked_similarity')),
    CONSTRAINT project_link_candidate_status_check
        CHECK (status IN ('pending', 'confirmed', 'rejected', 'expired')),
    CONSTRAINT project_link_candidate_confidence_check
        CHECK (confidence >= 0 AND confidence <= 1),
    CONSTRAINT project_link_candidate_evidence_shape CHECK (
        (evidence_field IS NULL AND evidence_start IS NULL AND evidence_end IS NULL)
        OR (evidence_field IN ('subject', 'body')
            AND evidence_start IS NOT NULL AND evidence_end IS NOT NULL
            AND evidence_start >= 0 AND evidence_end > evidence_start)),
    CONSTRAINT project_link_candidate_decided CHECK (
        (status = 'pending' AND decided_at IS NULL)
        OR (status <> 'pending' AND decided_at IS NOT NULL))
);

CREATE UNIQUE INDEX uq_project_link_candidate_pending
    ON project_link_candidate (activity_id) WHERE status = 'pending';
CREATE INDEX idx_project_link_candidate_project
    ON project_link_candidate (project_id) WHERE status = 'pending';
CREATE INDEX idx_project_link_candidate_proposal
    ON project_link_candidate (proposal_id) WHERE proposal_id IS NOT NULL;

CREATE TRIGGER project_link_candidate_touch BEFORE UPDATE ON project_link_candidate
    FOR EACH ROW EXECUTE FUNCTION set_updated_at_bump_version();

GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE project_link_candidate TO margince_app;
