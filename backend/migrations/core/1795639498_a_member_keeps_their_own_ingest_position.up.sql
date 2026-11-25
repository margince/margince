-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- A member-owned source keeps one position per member, and a save names the
-- version it read so a stale writer is refused. A single-stream source keeps its
-- one position under the nil member id, as before. The table holds a handful of
-- rows, one per unit and source, so rebuilding its key is cheap.
SET LOCAL lock_timeout = '3s';

ALTER TABLE extension_ingest_position
    ADD COLUMN member_id uuid NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
    ADD COLUMN version bigint NOT NULL DEFAULT 1,
    ADD CONSTRAINT extension_ingest_position_version_bound CHECK (version BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID,
    DROP CONSTRAINT extension_ingest_position_pkey,
    ADD CONSTRAINT extension_ingest_position_pkey PRIMARY KEY (unit, source_system, member_id);
