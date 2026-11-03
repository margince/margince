-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '3s';

ALTER TABLE assurance_exception ADD COLUMN input_changed_at timestamptz;

ALTER TABLE assurance_run DROP COLUMN read_through_generation;

DROP TABLE assurance_input_change;

DROP SEQUENCE assurance_input_generation_seq;
