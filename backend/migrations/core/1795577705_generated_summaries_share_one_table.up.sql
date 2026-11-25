-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- Five tables each cached one generated summary of a record for one reader.
-- They become one table, and the kind says which summary a row is.
--
-- Each parent keeps a foreign key column of its own, so a deleted company,
-- contact or deal still takes its summaries with it.
SET LOCAL lock_timeout = '3s';

CREATE TABLE generated_summary (
    user_id uuid NOT NULL,
    kind text NOT NULL,
    company_id uuid,
    contact_id uuid,
    deal_id uuid,
    fingerprint text NOT NULL,
    payload jsonb NOT NULL,
    generated_by text NOT NULL,
    generated_at timestamptz DEFAULT now() NOT NULL,
    CONSTRAINT generated_summary_reader_kind_record_key
        UNIQUE NULLS NOT DISTINCT (user_id, kind, company_id, contact_id, deal_id),
    CONSTRAINT generated_summary_kind_check CHECK (kind IN (
        'company_brief', 'company_dossier', 'company_growth_fit', 'contact_brief', 'deal_status_card')),
    CONSTRAINT generated_summary_kind_names_its_record CHECK (
        num_nonnulls(company_id, contact_id, deal_id) = 1
        AND (contact_id IS NOT NULL) = (kind = 'contact_brief')
        AND (deal_id IS NOT NULL) = (kind = 'deal_status_card')),
    CONSTRAINT generated_summary_generated_by_check CHECK (generated_by IN ('model', 'deterministic')),
    CONSTRAINT generated_summary_user_fkey FOREIGN KEY (user_id) REFERENCES app_user(id) ON DELETE CASCADE,
    CONSTRAINT generated_summary_company_fkey FOREIGN KEY (company_id) REFERENCES company(id) ON DELETE CASCADE,
    CONSTRAINT generated_summary_contact_fkey FOREIGN KEY (contact_id) REFERENCES contact(id) ON DELETE CASCADE,
    CONSTRAINT generated_summary_deal_fkey FOREIGN KEY (deal_id) REFERENCES deal(id) ON DELETE CASCADE
);

COMMENT ON TABLE generated_summary IS 'Read-model cache of generated summaries, keyed per READER: no summary crosses readers, whatever their masks or scope.';

CREATE INDEX generated_summary_company_ix ON generated_summary (company_id) WHERE company_id IS NOT NULL;
CREATE INDEX generated_summary_contact_ix ON generated_summary (contact_id) WHERE contact_id IS NOT NULL;
CREATE INDEX generated_summary_deal_ix ON generated_summary (deal_id) WHERE deal_id IS NOT NULL;

GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE generated_summary TO margince_app;

CREATE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."generated_summary" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"fingerprint":20000,"generated_by":20000,"kind":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"fingerprint":20000,"generated_by":20000,"kind":20000}');
CREATE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."generated_summary" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"fingerprint":20000,"generated_by":20000,"kind":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"fingerprint":20000,"generated_by":20000,"kind":20000}');

-- The rows are carried over, so no reader pays a model call for a summary
-- that was already written for them.
INSERT INTO generated_summary (user_id, kind, company_id, fingerprint, payload, generated_by, generated_at)
SELECT user_id, 'company_brief', company_id, fingerprint, payload, generated_by, generated_at
  FROM company_brief;

INSERT INTO generated_summary (user_id, kind, company_id, fingerprint, payload, generated_by, generated_at)
SELECT user_id, 'company_dossier', company_id, fingerprint, payload, generated_by, generated_at
  FROM company_dossier;

INSERT INTO generated_summary (user_id, kind, company_id, fingerprint, payload, generated_by, generated_at)
SELECT user_id, 'company_growth_fit', company_id, fingerprint, payload, generated_by, generated_at
  FROM company_growth_fit;

INSERT INTO generated_summary (user_id, kind, contact_id, fingerprint, payload, generated_by, generated_at)
SELECT user_id, 'contact_brief', contact_id, fingerprint, payload, generated_by, generated_at
  FROM contact_brief;

INSERT INTO generated_summary (user_id, kind, deal_id, fingerprint, payload, generated_by, generated_at)
SELECT user_id, 'deal_status_card', deal_id, fingerprint, payload, generated_by, generated_at
  FROM deal_status_card;

DROP TABLE company_brief;
DROP TABLE company_dossier;
DROP TABLE company_growth_fit;
DROP TABLE contact_brief;
DROP TABLE deal_status_card;
