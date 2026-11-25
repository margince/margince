-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '3s';

DELETE FROM extension_ingest_position WHERE member_id <> '00000000-0000-0000-0000-000000000000';

ALTER TABLE extension_ingest_position
    DROP CONSTRAINT extension_ingest_position_pkey,
    DROP COLUMN member_id,
    DROP COLUMN version,
    ADD CONSTRAINT extension_ingest_position_pkey PRIMARY KEY (unit, source_system);
