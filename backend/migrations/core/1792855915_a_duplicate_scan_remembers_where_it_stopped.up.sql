-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- The nightly duplicate scan's watermark, one per table it reads: the last row
-- it inspected, by updated_at and then id. The next pass starts after it. The
-- identifier tables are read too, so a merge that moves only an address, a
-- number or a domain still names its record.
SET LOCAL lock_timeout = '3s';

CREATE TABLE dedupe_scan_state (
    stream text NOT NULL,
    scanned_through_at timestamp with time zone NOT NULL,
    scanned_through_id uuid NOT NULL,

    CONSTRAINT dedupe_scan_state_pkey PRIMARY KEY (stream),
    CONSTRAINT dedupe_scan_state_stream_check
        CHECK (stream IN ('company', 'company_domain', 'contact', 'contact_email', 'contact_phone'))
);
