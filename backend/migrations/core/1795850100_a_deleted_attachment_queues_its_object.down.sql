-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '3s';
DROP TRIGGER attachment_deleted_object ON attachment;
DROP FUNCTION queue_deleted_attachment_object();
CREATE OR REPLACE FUNCTION record_stored_object_intent(requested_kind text, requested_key text)
RETURNS void LANGUAGE plpgsql AS $$BEGIN
    INSERT INTO stored_object_intent (storage_key, kind)
    VALUES (requested_key, requested_kind)
    ON CONFLICT (storage_key) DO UPDATE
       SET kind = excluded.kind, recorded_at = now()
       WHERE stored_object_intent.reaping_since IS NULL;
    IF NOT FOUND THEN
        RAISE EXCEPTION 'The stored file has expired; upload it again' USING ERRCODE = '55000';
    END IF;
END;$$;

DROP FUNCTION record_stored_object_intent(text,text,boolean);
