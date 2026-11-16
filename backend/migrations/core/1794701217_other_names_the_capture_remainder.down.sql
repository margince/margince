-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '5s';

ALTER TABLE activity DROP CONSTRAINT activity_capture_label_check;
ALTER TABLE activity ADD CONSTRAINT activity_capture_label_check
    CHECK (capture_label IN ('commitment', 'meeting', 'other', 'noise')) NOT VALID;

DROP FUNCTION public.rename_legacy_capture_labels(uuid[]);
