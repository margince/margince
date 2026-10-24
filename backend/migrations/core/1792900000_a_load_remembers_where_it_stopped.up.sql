-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- Where a unit's bulk load of one source stopped, so a restart carries on from
-- there. The position is the unit's own cursor text; the core stores it and
-- never reads it. One row per unit per source, replaced by each saved batch.
SET LOCAL lock_timeout = '3s';

CREATE TABLE extension_ingest_position (
    unit text NOT NULL,
    source_system text NOT NULL,
    position text NOT NULL,
    saved_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (unit, source_system),
    CONSTRAINT extension_ingest_position_size CHECK (position <> '' AND octet_length(position) <= 512)
);
