-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '5s';

ALTER TABLE activity DROP CONSTRAINT activity_capture_label_check;
ALTER TABLE activity ADD CONSTRAINT activity_capture_label_check
    CHECK (capture_label IN ('commitment', 'meeting', 'other', 'noise')) NOT VALID;

CREATE FUNCTION public.rename_legacy_capture_labels(subjects uuid[]) RETURNS integer
LANGUAGE plpgsql AS $$DECLARE
    moving text := current_setting('margince.last_activity_move', true);
    changed integer;
BEGIN
    PERFORM set_config('margince.last_activity_move', 'on', true);
    UPDATE activity SET capture_label = 'other'
     WHERE id = ANY(subjects) AND capture_label = 'noise' AND restricted_at IS NULL;
    GET DIAGNOSTICS changed = ROW_COUNT;
    PERFORM set_config('margince.last_activity_move', coalesce(moving, ''), true);
    RETURN changed;
EXCEPTION WHEN OTHERS THEN
    PERFORM set_config('margince.last_activity_move', coalesce(moving, ''), true);
    RAISE;
END;
$$;
