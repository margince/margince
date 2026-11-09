SET LOCAL lock_timeout = '3s';

DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM raw_capture WHERE payload_object IS NOT NULL) THEN
        RAISE EXCEPTION 'Restore externally stored originals inline before removing their references';
    END IF;
END $$;
DROP TRIGGER raw_original_object_retired ON raw_capture;
DROP FUNCTION retire_raw_original_object();
DROP FUNCTION record_stored_object_intent(text, text);
ALTER TABLE raw_capture DROP CONSTRAINT raw_capture_payload_object_shape;
ALTER TABLE raw_capture DROP COLUMN payload_object;
ALTER TABLE raw_capture DROP COLUMN payload_offload_checked_at;
