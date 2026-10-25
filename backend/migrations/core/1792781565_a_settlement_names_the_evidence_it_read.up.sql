-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '2s';

-- The request and answers a settlement verdict read, as id:version pairs. A
-- verdict commits only while they are unchanged, and a settled verdict that
-- carries them is judged again when a newer answer arrives. NULL on verdicts
-- recorded before this column, which the reviewed repair handles instead.
ALTER TABLE activity_request_settlement ADD COLUMN IF NOT EXISTS judged_evidence text;

-- When a reviewed repair asked for an older settled verdict to be judged
-- again. The next verdict the pass records clears it.
ALTER TABLE activity_request_settlement ADD COLUMN IF NOT EXISTS revalidation_requested_at timestamptz;

-- The settlement evidence generation. Every write that can add, change or
-- remove an answer to a request takes a shared lock on the conversation it
-- belongs to: its thread, its correspondent's address, or the contact it is
-- filed under. An UPDATE locks the keys of the row before and after, and a
-- DELETE the keys of the row it removes, so moving an answer away from a
-- conversation is serialized like adding one. A settlement verdict takes the
-- same locks exclusively before its final evidence check, so an answer is
-- either seen by that check or changed after the verdict commits.
CREATE OR REPLACE FUNCTION settlement_evidence_key(scope text, value text) RETURNS bigint
    LANGUAGE sql IMMUTABLE AS $$ SELECT hashtextextended('margince.settlement.' || scope || ':' || value, 0) $$;

-- Takes each key once and in key order, the order the verdict takes them in.
CREATE OR REPLACE FUNCTION settlement_evidence_share(keys bigint[]) RETURNS void
    LANGUAGE plpgsql AS $$
DECLARE
    k bigint;
BEGIN
    FOR k IN SELECT DISTINCT u FROM unnest(keys) u WHERE u IS NOT NULL ORDER BY u LOOP
        PERFORM pg_advisory_xact_lock_shared(k);
    END LOOP;
END $$;

CREATE OR REPLACE FUNCTION settlement_evidence_activity_keys(row_value activity) RETURNS bigint[]
    LANGUAGE sql IMMUTABLE AS $$
    SELECT CASE WHEN row_value.kind IN ('email', 'message', 'call', 'meeting') THEN ARRAY[
        settlement_evidence_key('thread', row_value.thread_key),
        settlement_evidence_key('address', lower(btrim(row_value.counterparty_email)))] END
$$;

CREATE OR REPLACE FUNCTION settlement_evidence_share_activity() RETURNS trigger
    LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        PERFORM settlement_evidence_share(settlement_evidence_activity_keys(OLD));
        RETURN OLD;
    END IF;
    IF TG_OP = 'UPDATE' THEN
        PERFORM settlement_evidence_share(settlement_evidence_activity_keys(OLD) || settlement_evidence_activity_keys(NEW));
    ELSE
        PERFORM settlement_evidence_share(settlement_evidence_activity_keys(NEW));
    END IF;
    RETURN NEW;
END $$;

CREATE OR REPLACE FUNCTION settlement_evidence_participant_keys(row_value activity_participant) RETURNS bigint[]
    LANGUAGE sql IMMUTABLE AS $$
    SELECT ARRAY[settlement_evidence_key('contact', row_value.contact_id::text),
                 settlement_evidence_key('address', lower(btrim(row_value.address)))]
$$;

CREATE OR REPLACE FUNCTION settlement_evidence_share_participant() RETURNS trigger
    LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        PERFORM settlement_evidence_share(settlement_evidence_participant_keys(OLD));
        RETURN OLD;
    END IF;
    IF TG_OP = 'UPDATE' THEN
        PERFORM settlement_evidence_share(settlement_evidence_participant_keys(OLD) || settlement_evidence_participant_keys(NEW));
    ELSE
        PERFORM settlement_evidence_share(settlement_evidence_participant_keys(NEW));
    END IF;
    RETURN NEW;
END $$;

CREATE OR REPLACE FUNCTION settlement_evidence_share_link() RETURNS trigger
    LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        PERFORM settlement_evidence_share(ARRAY[settlement_evidence_key('contact', OLD.contact_id::text)]);
        RETURN OLD;
    END IF;
    IF TG_OP = 'UPDATE' THEN
        PERFORM settlement_evidence_share(ARRAY[settlement_evidence_key('contact', OLD.contact_id::text),
                                                settlement_evidence_key('contact', NEW.contact_id::text)]);
    ELSE
        PERFORM settlement_evidence_share(ARRAY[settlement_evidence_key('contact', NEW.contact_id::text)]);
    END IF;
    RETURN NEW;
END $$;

CREATE OR REPLACE FUNCTION settlement_evidence_contact_email_keys(row_value contact_email) RETURNS bigint[]
    LANGUAGE sql IMMUTABLE AS $$
    SELECT ARRAY[settlement_evidence_key('contact', row_value.contact_id::text),
                 settlement_evidence_key('address', lower(btrim(row_value.email)))]
$$;

CREATE OR REPLACE FUNCTION settlement_evidence_share_contact_email() RETURNS trigger
    LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        PERFORM settlement_evidence_share(settlement_evidence_contact_email_keys(OLD));
        RETURN OLD;
    END IF;
    IF TG_OP = 'UPDATE' THEN
        PERFORM settlement_evidence_share(settlement_evidence_contact_email_keys(OLD) || settlement_evidence_contact_email_keys(NEW));
    ELSE
        PERFORM settlement_evidence_share(settlement_evidence_contact_email_keys(NEW));
    END IF;
    RETURN NEW;
END $$;

DROP TRIGGER IF EXISTS settlement_evidence_share ON activity;
CREATE TRIGGER settlement_evidence_share BEFORE INSERT OR UPDATE OR DELETE ON activity
    FOR EACH ROW EXECUTE FUNCTION settlement_evidence_share_activity();
DROP TRIGGER IF EXISTS settlement_evidence_share ON activity_participant;
CREATE TRIGGER settlement_evidence_share BEFORE INSERT OR UPDATE OR DELETE ON activity_participant
    FOR EACH ROW EXECUTE FUNCTION settlement_evidence_share_participant();
DROP TRIGGER IF EXISTS settlement_evidence_share ON activity_link;
CREATE TRIGGER settlement_evidence_share BEFORE INSERT OR UPDATE OR DELETE ON activity_link
    FOR EACH ROW EXECUTE FUNCTION settlement_evidence_share_link();
DROP TRIGGER IF EXISTS settlement_evidence_share ON contact_email;
CREATE TRIGGER settlement_evidence_share BEFORE INSERT OR UPDATE OR DELETE ON contact_email
    FOR EACH ROW EXECUTE FUNCTION settlement_evidence_share_contact_email();
