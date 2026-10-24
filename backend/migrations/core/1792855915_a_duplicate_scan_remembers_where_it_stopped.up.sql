-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- The nightly duplicate scan's watermark: the last record of each type it
-- inspected, by updated_at and then id. The next pass starts after it.
SET LOCAL lock_timeout = '3s';

CREATE TABLE dedupe_scan_state (
    entity_type text NOT NULL,
    scanned_through_at timestamp with time zone NOT NULL,
    scanned_through_id uuid NOT NULL,

    CONSTRAINT dedupe_scan_state_pkey PRIMARY KEY (entity_type),
    CONSTRAINT dedupe_scan_state_entity_type_check CHECK (entity_type IN ('company', 'contact'))
);
