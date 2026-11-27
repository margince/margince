-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- The one-sentence status How it stands shows under its verdict, rewritten in
-- the background when an activity lands on the record. Shared by every reader:
-- a reader sees it only while they may open every activity it cites.
SET LOCAL lock_timeout = '3s';

CREATE TABLE standing_line (
    entity_type text NOT NULL,
    entity_id uuid NOT NULL,
    text text NOT NULL,
    evidence uuid[] NOT NULL,
    language text NOT NULL,
    fingerprint text NOT NULL,
    written_at timestamptz DEFAULT now() NOT NULL,
    CONSTRAINT standing_line_pkey PRIMARY KEY (entity_type, entity_id),
    CONSTRAINT standing_line_entity_type_check CHECK (entity_type IN ('contact', 'company', 'project')),
    CONSTRAINT standing_line_cites_something CHECK (cardinality(evidence) > 0)
);

COMMENT ON TABLE standing_line IS 'Derived status sentence per record, shown only to a reader who may open every cited activity.';

CREATE INDEX standing_line_evidence_ix ON standing_line USING gin (evidence);

GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE standing_line TO margince_app;

CREATE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."standing_line" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"entity_type":20000,"fingerprint":20000,"language":20000,"text":2000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"entity_type":20000,"fingerprint":20000,"language":20000,"text":2000}');
CREATE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."standing_line" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"entity_type":20000,"fingerprint":20000,"language":20000,"text":2000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"entity_type":20000,"fingerprint":20000,"language":20000,"text":2000}');
