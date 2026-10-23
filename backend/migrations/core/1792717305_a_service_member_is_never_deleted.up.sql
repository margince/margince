-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '2s';

-- A MEMBER THAT OWNS AN UNATTENDED FEED, NOT A PERSON.
--
-- An extension's scheduled feed lands records as a member, and the credential it
-- polls with is filed under that member in extension_secret. When the member was
-- a colleague, the day they left stopped the feed and the day their row was
-- deleted took the credential with it (extension_secret.user_id cascades). A
-- service member is the member that outlives every colleague.
--
-- `service_extension` names the one extension unit the member serves, and it is
-- what the consent check compares: a unit may act as a service member only if
-- the member was declared for that unit.
--
-- It is an is_agent row with no password, so the existing guarantees carry over:
-- sign-in refuses an agent, and app_user_agent_is_full keeps it a full seat. It
-- is not a human-led agent, which is why it is a separate column and not the
-- bare flag. The seat meter skips is_agent rows, so a service member is free of
-- the licence seat limit.
ALTER TABLE app_user ADD COLUMN service_extension text;

ALTER TABLE app_user
    ADD CONSTRAINT app_user_service_is_a_passwordless_agent
    CHECK (service_extension IS NULL OR (is_agent AND password_hash IS NULL AND service_extension <> '')) NOT VALID;

-- ARCHIVED, NEVER DELETED. The delete would cascade to the member's
-- extension_secret rows and strand every audit row that names the member as an
-- actor. Offboarding stops a feed by archiving its member, which is a decision
-- somebody makes, and not by a cascade nobody meant.
CREATE FUNCTION refuse_service_member_delete() RETURNS trigger
    LANGUAGE plpgsql AS $$BEGIN
    IF OLD.service_extension IS NOT NULL THEN
        RAISE EXCEPTION 'a service member is archived, never deleted'
            USING ERRCODE = 'restrict_violation';
    END IF;
    RETURN OLD;
END;
$$;

CREATE TRIGGER app_user_service_never_deleted BEFORE DELETE ON app_user
    FOR EACH ROW EXECUTE FUNCTION refuse_service_member_delete();
