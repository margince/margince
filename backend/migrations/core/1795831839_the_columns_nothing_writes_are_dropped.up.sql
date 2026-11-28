-- Nothing writes or reads these columns; only their own key, index or check
-- names them. No writer ever set a team voice profile, so its owner check is
-- rebuilt to admit only a user's or the workspace's, and the next file
-- validates it. A consent wording's window and jurisdiction were never set.
-- A grant's uniqueness moves to the index the previous file built.
SET LOCAL lock_timeout = '3s';

ALTER TABLE oauth_client DROP COLUMN last_used_at;
ALTER TABLE oauth_grant DROP COLUMN lent_passport_id;
ALTER TABLE oauth_authorization_code DROP COLUMN lent_passport_id;
ALTER TABLE communication_review
    DROP CONSTRAINT communication_review_supersession_shape,
    DROP COLUMN superseded_by;
ALTER TABLE conversation_claim DROP COLUMN corrected_by_user_id;
ALTER TABLE finance_external_customer DROP COLUMN archived_at;
ALTER TABLE contact_acquisition_evidence DROP COLUMN purpose_claimed;

ALTER TABLE role_assignment DROP COLUMN team_id;
ALTER INDEX uq_role_assignment_member RENAME TO uq_role_assignment;

ALTER TABLE voice_profile
    DROP CONSTRAINT voice_profile_scope_owner_check,
    DROP COLUMN team_id,
    ADD CONSTRAINT voice_profile_scope_owner_check CHECK (
        (scope = 'user' AND owner_id IS NOT NULL)
        OR (scope = 'workspace' AND owner_id IS NULL)) NOT VALID;

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
       OR NEW.locale IS DISTINCT FROM OLD.locale
       OR NEW.channel IS DISTINCT FROM OLD.channel
       OR NEW.purpose_id IS DISTINCT FROM OLD.purpose_id THEN
        RAISE EXCEPTION 'consent_text_version % is published and its wording cannot change; publish a new version instead', OLD.id
            USING ERRCODE = 'restrict_violation';
    END IF;
    RETURN NEW;
END;
$$;

ALTER TABLE consent_text_version
    DROP CONSTRAINT consent_text_version_window,
    DROP COLUMN jurisdiction,
    DROP COLUMN effective_from,
    DROP COLUMN effective_until;
