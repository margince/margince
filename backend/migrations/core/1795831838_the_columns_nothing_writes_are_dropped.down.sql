-- The columns come back empty, with their keys, indexes and checks; a key or
-- check comes back NOT VALID, as a rollback adds it. A published wording's
-- effective_until can again be closed by the application.
SET LOCAL lock_timeout = '3s';

ALTER TABLE consent_text_version
    ADD COLUMN jurisdiction text,
    ADD COLUMN effective_from timestamptz,
    ADD COLUMN effective_until timestamptz,
    ADD CONSTRAINT consent_text_version_window CHECK (
        effective_until IS NULL OR effective_from IS NULL
        OR effective_until > effective_from) NOT VALID;
GRANT UPDATE (effective_until) ON TABLE consent_text_version TO margince_app;

CREATE OR REPLACE FUNCTION consent_text_version_is_immutable()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF OLD.published_at IS NULL THEN
        RETURN NEW;
    END IF;
    IF NEW.key IS DISTINCT FROM OLD.key
       OR NEW.version IS DISTINCT FROM OLD.version
       OR NEW.subject_line IS DISTINCT FROM OLD.subject_line
       OR NEW.body IS DISTINCT FROM OLD.body
       OR NEW.disclosures IS DISTINCT FROM OLD.disclosures
       OR NEW.content_hash IS DISTINCT FROM OLD.content_hash
       OR NEW.published_at IS DISTINCT FROM OLD.published_at
       OR NEW.effective_from IS DISTINCT FROM OLD.effective_from
       OR NEW.jurisdiction IS DISTINCT FROM OLD.jurisdiction
       OR NEW.locale IS DISTINCT FROM OLD.locale
       OR NEW.channel IS DISTINCT FROM OLD.channel
       OR NEW.purpose_id IS DISTINCT FROM OLD.purpose_id THEN
        RAISE EXCEPTION 'consent_text_version % is published and its wording cannot change; publish a new version instead', OLD.id
            USING ERRCODE = 'restrict_violation';
    END IF;
    RETURN NEW;
END;
$$;

ALTER TABLE voice_profile
    DROP CONSTRAINT voice_profile_scope_owner_check,
    ADD COLUMN team_id uuid,
    ADD CONSTRAINT voice_profile_scope_owner_check CHECK (
        (scope = 'user' AND owner_id IS NOT NULL AND team_id IS NULL)
        OR (scope = 'team' AND owner_id IS NULL AND team_id IS NOT NULL)
        OR (scope = 'workspace' AND owner_id IS NULL AND team_id IS NULL)) NOT VALID,
    ADD CONSTRAINT voice_profile_team_fkey FOREIGN KEY (team_id)
        REFERENCES team(id) ON DELETE RESTRICT NOT VALID;
CREATE INDEX voice_profile_team_fk ON voice_profile (team_id) WHERE team_id IS NOT NULL;

DROP INDEX uq_role_assignment;
ALTER TABLE role_assignment
    ADD COLUMN team_id uuid,
    ADD CONSTRAINT role_assignment_team_id_fkey FOREIGN KEY (team_id)
        REFERENCES team(id) ON DELETE CASCADE NOT VALID;
CREATE INDEX idx_role_assignment_team ON role_assignment (team_id);
CREATE UNIQUE INDEX uq_role_assignment ON role_assignment (role_id, user_id, team_id) NULLS NOT DISTINCT;

ALTER TABLE contact_acquisition_evidence ADD COLUMN purpose_claimed text;
ALTER TABLE finance_external_customer ADD COLUMN archived_at timestamptz;
ALTER TABLE conversation_claim
    ADD COLUMN corrected_by_user_id uuid,
    ADD CONSTRAINT conversation_claim_corrector_fkey FOREIGN KEY (corrected_by_user_id)
        REFERENCES app_user(id) ON DELETE SET NULL NOT VALID;
ALTER TABLE communication_review
    ADD COLUMN superseded_by uuid,
    ADD CONSTRAINT communication_review_superseded_by_fkey FOREIGN KEY (superseded_by)
        REFERENCES communication_review(id) ON DELETE RESTRICT NOT VALID,
    ADD CONSTRAINT communication_review_supersession_shape
        CHECK ((state = 'superseded') = (superseded_by IS NOT NULL)) NOT VALID;
ALTER TABLE oauth_authorization_code
    ADD COLUMN lent_passport_id uuid,
    ADD CONSTRAINT oauth_code_lent_passport_fkey FOREIGN KEY (lent_passport_id)
        REFERENCES passport(id) ON DELETE SET NULL (lent_passport_id) NOT VALID;
CREATE INDEX oauth_code_lent_passport_ix ON oauth_authorization_code (lent_passport_id);
ALTER TABLE oauth_grant
    ADD COLUMN lent_passport_id uuid,
    ADD CONSTRAINT oauth_grant_lent_passport_fkey FOREIGN KEY (lent_passport_id)
        REFERENCES passport(id) ON DELETE SET NULL NOT VALID;
CREATE INDEX oauth_grant_lent_passport_ix ON oauth_grant (lent_passport_id);
ALTER TABLE oauth_client ADD COLUMN last_used_at timestamptz;
