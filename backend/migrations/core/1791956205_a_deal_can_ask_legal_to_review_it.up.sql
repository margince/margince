-- A deal owner asks legal to review a deal, and a reviewer answers.
--
-- The review carries a packet built from the deal at the moment of the
-- request: its company, value, stage, close date, current offer, stage
-- history and the contract, legal and offer documents. The packet keeps the
-- facts the reviewer judged, so a later edit to the deal cannot change what
-- an approval was given on. Documents are frozen as ids and states only; their
-- names and bytes are read live, so an erasure of a file reaches the packet.
--
-- A stage may require an approval before a deal leaves it: the exit-criterion
-- kind legal_reviewed, settled only by an approved review, through the
-- evidence source legal_review.
--
-- The three widened checks are added NOT VALID and validated in the file beside
-- this one, so the scan of existing rows runs under SHARE UPDATE EXCLUSIVE.
SET LOCAL lock_timeout = '5s';

CREATE TABLE legal_review (
    id uuid DEFAULT uuidv7() NOT NULL,
    deal_id uuid NOT NULL,
    status text DEFAULT 'requested' NOT NULL,
    requested_by uuid,
    note text,
    -- The changes_needed review this one asks again after, so the reviewer
    -- reads the earlier answer beside the new request.
    previous_review_id uuid,
    packet jsonb NOT NULL,
    -- The queue's sort keys, frozen with the packet. value_minor is the deal's
    -- amount priced in the base currency on the request day, because an open
    -- deal carries no base amount of its own.
    value_minor bigint,
    value_currency character(3),
    expected_close_date date,
    decided_by uuid,
    decided_at timestamptz,
    comments text,
    captured_by text NOT NULL,
    version bigint DEFAULT 1 NOT NULL,
    created_at timestamptz DEFAULT now() NOT NULL,
    updated_at timestamptz DEFAULT now() NOT NULL,

    CONSTRAINT legal_review_pkey PRIMARY KEY (id),
    CONSTRAINT legal_review_status_check
        CHECK (status IN ('requested', 'approved', 'changes_needed', 'withdrawn')),
    -- A decision has a moment exactly when it is an outcome. decided_by may
    -- go NULL later, when the deciding seat is deleted.
    CONSTRAINT legal_review_decided_shape
        CHECK ((status IN ('approved', 'changes_needed')) = (decided_at IS NOT NULL)),
    CONSTRAINT legal_review_value_pair
        CHECK ((value_minor IS NULL) = (value_currency IS NULL)),
    CONSTRAINT legal_review_note_bounded
        CHECK (note IS NULL OR length(note) <= 4000),
    CONSTRAINT legal_review_comments_bounded
        CHECK (comments IS NULL OR length(comments) <= 8000),
    CONSTRAINT legal_review_version_js_safe
        CHECK (version >= '-9007199254740991'::bigint AND version <= '9007199254740991'::bigint),
    CONSTRAINT legal_review_value_minor_js_safe
        CHECK (value_minor >= '-9007199254740991'::bigint AND value_minor <= '9007199254740991'::bigint)
);

ALTER TABLE legal_review
    ADD CONSTRAINT legal_review_deal_id_fkey FOREIGN KEY (deal_id)
    REFERENCES deal(id) ON DELETE CASCADE;
ALTER TABLE legal_review
    ADD CONSTRAINT legal_review_previous_review_id_fkey FOREIGN KEY (previous_review_id)
    REFERENCES legal_review(id) ON DELETE SET NULL;
ALTER TABLE legal_review
    ADD CONSTRAINT legal_review_requested_by_fkey FOREIGN KEY (requested_by)
    REFERENCES app_user(id) ON DELETE SET NULL;
ALTER TABLE legal_review
    ADD CONSTRAINT legal_review_decided_by_fkey FOREIGN KEY (decided_by)
    REFERENCES app_user(id) ON DELETE SET NULL;

-- One open request per deal: a second one would ask legal the same question twice.
CREATE UNIQUE INDEX uq_legal_review_open_deal ON legal_review (deal_id) WHERE status = 'requested';
CREATE INDEX idx_legal_review_deal ON legal_review (deal_id, created_at DESC);
-- The reviewer queue's order: the largest value first, then the nearest close.
CREATE INDEX idx_legal_review_queue ON legal_review
    (value_minor DESC NULLS LAST, expected_close_date ASC NULLS LAST, created_at, id)
    WHERE status = 'requested';
CREATE INDEX idx_legal_review_decided ON legal_review (decided_at DESC, id DESC)
    WHERE decided_at IS NOT NULL;
CREATE INDEX idx_legal_review_previous ON legal_review (previous_review_id)
    WHERE previous_review_id IS NOT NULL;
CREATE INDEX idx_legal_review_requested_by ON legal_review (requested_by)
    WHERE requested_by IS NOT NULL;
CREATE INDEX idx_legal_review_decided_by ON legal_review (decided_by)
    WHERE decided_by IS NOT NULL;

CREATE TRIGGER trg_legal_review_updated
    BEFORE UPDATE ON legal_review
    FOR EACH ROW EXECUTE FUNCTION set_updated_at_bump_version();

-- The two verbs a deal's audit trail records for the request and its withdrawal.
-- The decision is `resolve` on the review itself.
ALTER TABLE audit_log DROP CONSTRAINT audit_log_action_check;

ALTER TABLE audit_log ADD CONSTRAINT audit_log_action_check
    CHECK (action IN ('create', 'update', 'archive', 'merge', 'promote', 'restore',
                      'export', 'erase', 'assign', 'advance_stage', 'advance_phase',
                      'approve', 'reject', 'consent_grant', 'consent_withdraw',
                      'activity_relink', 'record_share', 'record_unshare', 'resolve',
                      'demote', 'import', 'import_undo', 'disqualify', 'anonymize',
                      'send_email', 'reset_data', 'password_link_issued', 'connect',
                      'disconnect', 'schedule', 'reschedule', 'cancel', 'release',
                      'hold', 'expire', 'restrict', 'pin', 'accrue', 'pay', 'publish',
                      'pause', 'resume', 'close', 'invite', 'revoke', 'delete',
                      'place_legal_hold', 'lift_legal_hold',
                      'request_legal_review', 'withdraw_legal_review')) NOT VALID;

ALTER TABLE stage_exit_criterion DROP CONSTRAINT stage_exit_criterion_kind_check;
ALTER TABLE stage_exit_criterion ADD CONSTRAINT stage_exit_criterion_kind_check
    CHECK (kind IN ('buyer_confirmed', 'event_held', 'document_signed',
                    'role_identified', 'terms_accepted', 'custom', 'legal_reviewed')) NOT VALID;

ALTER TABLE deal_stage_evidence DROP CONSTRAINT deal_stage_evidence_source_type_check;
ALTER TABLE deal_stage_evidence ADD CONSTRAINT deal_stage_evidence_source_type_check
    CHECK (source_type IN ('activity', 'contract', 'legal_review')) NOT VALID;

-- Reviewing is a grant an admin hands to any seat. The admin role holds it;
-- every other system role is written the zero grant, so a role with no key
-- reads as one the backfill missed. Presence-guarded: a hand-set grant stays.
UPDATE role SET permissions = jsonb_set(
        permissions, '{objects,legal_review}',
        '{"create":false,"read":true,"update":true,"delete":false}'::jsonb, true)
    WHERE is_system
      AND key = 'admin'
      AND permissions ? 'objects'
      AND NOT (permissions -> 'objects') ? 'legal_review';

UPDATE role SET permissions = jsonb_set(
        permissions, '{objects,legal_review}',
        '{"create":false,"read":false,"update":false,"delete":false}'::jsonb, true)
    WHERE is_system
      AND permissions ? 'objects'
      AND NOT (permissions -> 'objects') ? 'legal_review';
