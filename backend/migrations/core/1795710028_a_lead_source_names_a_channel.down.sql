-- The list comes back with its built-in rows. Rows an administrator added are not restored.
SET LOCAL lock_timeout = '3s';

CREATE TABLE public.lead_source (
    id uuid DEFAULT public.uuidv7() NOT NULL,
    key text NOT NULL,
    label text NOT NULL,
    intent text DEFAULT 'neutral'::text NOT NULL,
    sort_order integer DEFAULT 0 NOT NULL,
    active boolean DEFAULT true NOT NULL,
    system boolean DEFAULT false NOT NULL,
    version bigint DEFAULT 1 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT lead_source_pkey PRIMARY KEY (id),
    CONSTRAINT lead_source_intent_check CHECK (intent IN ('high', 'neutral', 'low')),
    CONSTRAINT lead_source_key_shape CHECK (((key = lower(key)) AND (length(btrim(key)) > 0))),
    CONSTRAINT lead_source_label_present CHECK ((length(btrim(label)) > 0)),
    CONSTRAINT lead_source_version_js_safe CHECK (version BETWEEN -9007199254740991 AND 9007199254740991)
);
CREATE UNIQUE INDEX uq_lead_source_key ON public.lead_source USING btree (key);
CREATE TRIGGER trg_lead_source_updated BEFORE UPDATE ON public.lead_source
 FOR EACH ROW EXECUTE FUNCTION public.set_updated_at_bump_version();
CREATE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."lead_source" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"intent":20000,"key":20000,"label":120}')) EXECUTE FUNCTION public.refuse_oversized_text('{"intent":20000,"key":20000,"label":120}');
CREATE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."lead_source" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"intent":20000,"key":20000,"label":120}')) EXECUTE FUNCTION public.refuse_oversized_text('{"intent":20000,"key":20000,"label":120}');
GRANT SELECT, INSERT, DELETE, UPDATE ON TABLE public.lead_source TO margince_app;
INSERT INTO public.lead_source (key, label, intent, sort_order, active, system) VALUES
 ('manual', 'Created manually', 'neutral', 10, true, true),
 ('inbound', 'Inbound', 'high', 20, true, true),
 ('webform', 'Web form', 'high', 30, true, true),
 ('referral', 'Referral', 'high', 40, true, true),
 ('import', 'Import', 'low', 50, true, true),
 ('crawl', 'Crawl', 'low', 60, true, true),
 ('siteread', 'Website read', 'low', 70, true, true);

DROP TRIGGER lead_source_channel ON lead;
DROP FUNCTION move_lead_source_onto_its_channel();
ALTER TABLE lead DROP CONSTRAINT lead_source_check;
