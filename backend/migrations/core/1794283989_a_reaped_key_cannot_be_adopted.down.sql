-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '3s';

CREATE OR REPLACE FUNCTION record_stored_object_intent(requested_kind text, requested_key text)
RETURNS void LANGUAGE sql AS $$INSERT INTO stored_object_intent (storage_key, kind)
    VALUES (requested_key, requested_kind)
    ON CONFLICT (storage_key) DO UPDATE
       SET kind = excluded.kind, recorded_at = now();
$$;

ALTER TABLE stored_object_intent DROP COLUMN reaped_at;
ALTER TABLE stored_object_intent DROP COLUMN reaping_since;
