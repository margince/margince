-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- The five tables come back, each with the rows the shared table held for it.
SET LOCAL lock_timeout = '3s';

CREATE TABLE company_brief (
    id uuid DEFAULT uuidv7() NOT NULL,
    user_id uuid NOT NULL,
    company_id uuid NOT NULL,
    fingerprint text NOT NULL,
    generated_at timestamptz NOT NULL,
    generated_by text NOT NULL,
    payload jsonb NOT NULL,
    CONSTRAINT company_brief_pkey PRIMARY KEY (id),
    CONSTRAINT company_brief_user_id_company_id_key UNIQUE (user_id, company_id),
    CONSTRAINT company_brief_generated_by_check CHECK (generated_by = ANY (ARRAY['model'::text, 'deterministic'::text])),
    CONSTRAINT company_brief_company_fkey FOREIGN KEY (company_id) REFERENCES company(id) ON DELETE CASCADE,
    CONSTRAINT company_brief_user_id_fkey FOREIGN KEY (user_id) REFERENCES app_user(id) ON DELETE CASCADE
);
CREATE INDEX company_brief_company_ix ON company_brief (company_id);
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE company_brief TO margince_app;
CREATE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."company_brief" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"fingerprint":20000,"generated_by":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"fingerprint":20000,"generated_by":20000}');
CREATE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."company_brief" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"fingerprint":20000,"generated_by":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"fingerprint":20000,"generated_by":20000}');
INSERT INTO company_brief (user_id, company_id, fingerprint, payload, generated_by, generated_at)
SELECT user_id, company_id, fingerprint, payload, generated_by, generated_at
  FROM generated_summary WHERE kind = 'company_brief';

CREATE TABLE company_dossier (
    user_id uuid NOT NULL,
    company_id uuid NOT NULL,
    fingerprint text NOT NULL,
    payload jsonb NOT NULL,
    generated_by text NOT NULL,
    generated_at timestamptz DEFAULT now() NOT NULL,
    CONSTRAINT company_dossier_pkey PRIMARY KEY (user_id, company_id),
    CONSTRAINT company_dossier_generated_by_check CHECK (generated_by = ANY (ARRAY['model'::text, 'deterministic'::text])),
    CONSTRAINT company_dossier_company_fkey FOREIGN KEY (company_id) REFERENCES company(id) ON DELETE CASCADE,
    CONSTRAINT company_dossier_user_fkey FOREIGN KEY (user_id) REFERENCES app_user(id) ON DELETE CASCADE
);
COMMENT ON TABLE company_dossier IS 'Read-model cache (DOSS-DDL-1). Keyed per READER: no assembly crosses readers, whatever their masks.';
COMMENT ON COLUMN company_dossier.user_id IS 'The reader this assembly was generated for. Written into every read explicitly: the reader key is what keeps two readers'' assemblies apart, and the installation is the one workspace (ADR-0061).';
CREATE INDEX company_dossier_company_ix ON company_dossier (company_id);
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE company_dossier TO margince_app;
CREATE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."company_dossier" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"fingerprint":20000,"generated_by":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"fingerprint":20000,"generated_by":20000}');
CREATE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."company_dossier" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"fingerprint":20000,"generated_by":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"fingerprint":20000,"generated_by":20000}');
INSERT INTO company_dossier (user_id, company_id, fingerprint, payload, generated_by, generated_at)
SELECT user_id, company_id, fingerprint, payload, generated_by, generated_at
  FROM generated_summary WHERE kind = 'company_dossier';

CREATE TABLE company_growth_fit (
    user_id uuid NOT NULL,
    company_id uuid NOT NULL,
    fingerprint text NOT NULL,
    payload jsonb NOT NULL,
    generated_by text NOT NULL,
    generated_at timestamptz DEFAULT now() NOT NULL,
    CONSTRAINT company_growth_fit_pkey PRIMARY KEY (user_id, company_id),
    CONSTRAINT company_growth_fit_generated_by_check CHECK (generated_by = ANY (ARRAY['model'::text, 'deterministic'::text])),
    CONSTRAINT company_growth_fit_company_fkey FOREIGN KEY (company_id) REFERENCES company(id) ON DELETE CASCADE,
    CONSTRAINT company_growth_fit_user_fkey FOREIGN KEY (user_id) REFERENCES app_user(id) ON DELETE CASCADE
);
COMMENT ON TABLE company_growth_fit IS 'Read-model cache (DOSS-DDL-2). Per reader: it folds seat-dependent workspace context and makes recommendations.';
CREATE INDEX company_growth_fit_company_ix ON company_growth_fit (company_id);
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE company_growth_fit TO margince_app;
CREATE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."company_growth_fit" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"fingerprint":20000,"generated_by":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"fingerprint":20000,"generated_by":20000}');
CREATE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."company_growth_fit" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"fingerprint":20000,"generated_by":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"fingerprint":20000,"generated_by":20000}');
INSERT INTO company_growth_fit (user_id, company_id, fingerprint, payload, generated_by, generated_at)
SELECT user_id, company_id, fingerprint, payload, generated_by, generated_at
  FROM generated_summary WHERE kind = 'company_growth_fit';

CREATE TABLE contact_brief (
    user_id uuid NOT NULL,
    contact_id uuid NOT NULL,
    fingerprint text NOT NULL,
    payload jsonb NOT NULL,
    generated_by text NOT NULL,
    generated_at timestamptz DEFAULT now() NOT NULL,
    CONSTRAINT contact_brief_pkey PRIMARY KEY (user_id, contact_id),
    CONSTRAINT contact_brief_generated_by_check CHECK (generated_by = ANY (ARRAY['model'::text, 'deterministic'::text])),
    CONSTRAINT contact_brief_contact_fkey FOREIGN KEY (contact_id) REFERENCES contact(id) ON DELETE CASCADE,
    CONSTRAINT contact_brief_user_fkey FOREIGN KEY (user_id) REFERENCES app_user(id) ON DELETE CASCADE
);
COMMENT ON TABLE contact_brief IS 'Read-model cache (ADR-0097 D4), keyed per READER: no brief crosses readers, whatever their masks.';
CREATE INDEX contact_brief_contact_ix ON contact_brief (contact_id);
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE contact_brief TO margince_app;
CREATE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."contact_brief" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"fingerprint":20000,"generated_by":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"fingerprint":20000,"generated_by":20000}');
CREATE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."contact_brief" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"fingerprint":20000,"generated_by":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"fingerprint":20000,"generated_by":20000}');
INSERT INTO contact_brief (user_id, contact_id, fingerprint, payload, generated_by, generated_at)
SELECT user_id, contact_id, fingerprint, payload, generated_by, generated_at
  FROM generated_summary WHERE kind = 'contact_brief';

CREATE TABLE deal_status_card (
    user_id uuid NOT NULL,
    deal_id uuid NOT NULL,
    fingerprint text NOT NULL,
    payload jsonb NOT NULL,
    generated_by text NOT NULL,
    generated_at timestamptz DEFAULT now() NOT NULL,
    CONSTRAINT deal_status_card_pkey PRIMARY KEY (user_id, deal_id),
    CONSTRAINT deal_status_card_generated_by_check CHECK (generated_by = ANY (ARRAY['model'::text, 'deterministic'::text])),
    CONSTRAINT deal_status_card_deal_fkey FOREIGN KEY (deal_id) REFERENCES deal(id) ON DELETE CASCADE,
    CONSTRAINT deal_status_card_user_fkey FOREIGN KEY (user_id) REFERENCES app_user(id) ON DELETE CASCADE
);
COMMENT ON TABLE deal_status_card IS 'Read-model cache for the deal status card, keyed per READER: no card crosses readers, whatever their scope.';
CREATE INDEX deal_status_card_deal_ix ON deal_status_card (deal_id);
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE deal_status_card TO margince_app;
CREATE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."deal_status_card" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"fingerprint":20000,"generated_by":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"fingerprint":20000,"generated_by":20000}');
CREATE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."deal_status_card" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"fingerprint":20000,"generated_by":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"fingerprint":20000,"generated_by":20000}');
INSERT INTO deal_status_card (user_id, deal_id, fingerprint, payload, generated_by, generated_at)
SELECT user_id, deal_id, fingerprint, payload, generated_by, generated_at
  FROM generated_summary WHERE kind = 'deal_status_card';

DROP TABLE generated_summary;
