-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '3s';

CREATE TABLE source_attribution_repair (
    object_type text NOT NULL,
    object_id uuid NOT NULL,
    source_author_id uuid,
    source_author_name text,
    payload_hash text NOT NULL,
    source_revision bigint NOT NULL,
    batch_ref text NOT NULL,
    applied_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT source_attribution_repair_pkey PRIMARY KEY (object_type, object_id),
    CONSTRAINT source_attribution_repair_object_type CHECK (
        object_type IN ('activity', 'contact', 'company', 'deal', 'lead', 'project')),
    CONSTRAINT source_attribution_repair_revision CHECK (source_revision > 0));
