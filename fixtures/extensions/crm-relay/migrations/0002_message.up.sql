-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion
--
-- A posted test message waits here until the tick lands it, because a tool
-- call has a caller and only an unattended run may ingest.
CREATE TABLE ext.ext_crm_relay_message (
    id          uuid        NOT NULL DEFAULT gen_random_uuid() PRIMARY KEY,
    owner_id    uuid        NOT NULL,
    message_id  text        NOT NULL CHECK (length(message_id) BETWEEN 1 AND 200),
    document    jsonb,
    activity_id uuid,
    posted_at   timestamptz NOT NULL DEFAULT now(),
    landed_at   timestamptz,
    UNIQUE (owner_id, message_id)
);
