-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- What each extension keeps in the object store, so a unit can be held to a
-- quota. The store has no list or size call, so the core keeps the running
-- count and total size itself. Each Put and Delete updates this row in the
-- transaction that writes its audit row, which names the row by id. One row
-- per unit; an installation holds one workspace.
CREATE TABLE IF NOT EXISTS extension_object_usage (
    id uuid NOT NULL,
    unit text NOT NULL,
    objects bigint NOT NULL DEFAULT 0,
    bytes bigint NOT NULL DEFAULT 0,
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT extension_object_usage_pkey PRIMARY KEY (id),
    CONSTRAINT extension_object_usage_unit_key UNIQUE (unit),
    CONSTRAINT extension_object_usage_counted CHECK (objects >= 0 AND bytes >= 0)
);

CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."extension_object_usage" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"unit":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"unit":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."extension_object_usage" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"unit":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"unit":20000}');
