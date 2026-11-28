-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- Install the cleanup trigger before owner keys can cascade away a file row.
SET LOCAL lock_timeout = '3s';

-- Both adoption and deletion use the same ledger writer. Deletion renews the intent while keeping an
-- existing claim or tombstone; adoption must never take back condemned bytes.
CREATE FUNCTION record_stored_object_intent(requested_kind text, requested_key text, for_deletion boolean)
RETURNS void LANGUAGE plpgsql AS $$BEGIN
    INSERT INTO stored_object_intent (storage_key, kind)
    VALUES (requested_key, requested_kind)
    ON CONFLICT (storage_key) DO UPDATE
       SET kind = CASE WHEN for_deletion THEN stored_object_intent.kind ELSE excluded.kind END,
           recorded_at = CASE WHEN for_deletion THEN clock_timestamp() ELSE now() END
       WHERE for_deletion OR stored_object_intent.reaping_since IS NULL;
    IF NOT FOUND THEN
        RAISE EXCEPTION 'The stored file has expired; upload it again' USING ERRCODE = '55000';
    END IF;
END;$$;
CREATE OR REPLACE FUNCTION record_stored_object_intent(requested_kind text, requested_key text)
RETURNS void LANGUAGE plpgsql AS $$BEGIN
    PERFORM record_stored_object_intent(requested_kind, requested_key, false);
END;$$;

CREATE FUNCTION queue_deleted_attachment_object() RETURNS trigger LANGUAGE plpgsql AS $$BEGIN
    IF NOT OLD.bytes_withheld AND OLD.storage_key <> '' THEN
        PERFORM record_stored_object_intent('attachment', OLD.storage_key, true);
    END IF;
    RETURN OLD;
END;$$;
CREATE TRIGGER attachment_deleted_object AFTER DELETE ON attachment
    FOR EACH ROW EXECUTE FUNCTION queue_deleted_attachment_object();
