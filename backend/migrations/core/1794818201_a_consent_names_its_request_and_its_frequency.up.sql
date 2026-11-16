SET LOCAL lock_timeout = '3s';
-- A purpose says how often its mail goes out, and the subscription page states
-- it in the sentence a grant records. One closed word per frequency, so each
-- language can name it.
ALTER TABLE consent_purpose ADD COLUMN frequency text NOT NULL DEFAULT 'monthly';
ALTER TABLE consent_purpose ADD CONSTRAINT consent_purpose_frequency_check
    CHECK (frequency IN ('weekly', 'monthly', 'quarterly')) NOT VALID;

-- Where the rep who typed a contact in says it came from. The privacy page
-- prints it; `note` keeps the rep's own words beside it.
ALTER TABLE contact_acquisition_evidence ADD COLUMN stated_source text;
ALTER TABLE contact_acquisition_evidence ADD CONSTRAINT contact_acquisition_evidence_stated_source_check
    CHECK (stated_source IN ('met_in_person', 'business_card', 'referral', 'website', 'other')) NOT VALID;

-- A record-confirmation link that asks no news question. The recipient's
-- country decides at mint, and the page and the submit read it from the link.
ALTER TABLE confirm_token ADD COLUMN asks_news boolean NOT NULL DEFAULT true;

-- The subject's own request a consent-confirmation mail answers, kept with the
-- link and then with the grant it leads to.
ALTER TABLE confirm_token ADD COLUMN subject_request_id uuid;
ALTER TABLE confirm_token ADD CONSTRAINT confirm_token_subject_request_id_fkey
    FOREIGN KEY (subject_request_id) REFERENCES consent_qualifying_event (id) ON DELETE SET NULL NOT VALID;
ALTER TABLE consent_event ADD COLUMN subject_request_id uuid;
ALTER TABLE consent_event ADD CONSTRAINT consent_event_subject_request_id_fkey
    FOREIGN KEY (subject_request_id) REFERENCES consent_qualifying_event (id) ON DELETE SET NULL NOT VALID;

CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."consent_purpose" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"class":20000,"frequency":20000,"key":20000,"label":120}')) EXECUTE FUNCTION public.refuse_oversized_text('{"class":20000,"frequency":20000,"key":20000,"label":120}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."consent_purpose" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"class":20000,"frequency":20000,"key":20000,"label":120}')) EXECUTE FUNCTION public.refuse_oversized_text('{"class":20000,"frequency":20000,"key":20000,"label":120}');

CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."contact_acquisition_evidence" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"captured_by":20000,"kind":20000,"note":2000,"purpose_claimed":20000,"recorded_for_addresses":20000,"request_retracted_by":20000,"source_entity_type":20000,"stated_source":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"kind":20000,"note":2000,"purpose_claimed":20000,"recorded_for_addresses":20000,"request_retracted_by":20000,"source_entity_type":20000,"stated_source":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."contact_acquisition_evidence" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"captured_by":20000,"kind":20000,"note":2000,"purpose_claimed":20000,"recorded_for_addresses":20000,"request_retracted_by":20000,"source_entity_type":20000,"stated_source":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"captured_by":20000,"kind":20000,"note":2000,"purpose_claimed":20000,"recorded_for_addresses":20000,"request_retracted_by":20000,"source_entity_type":20000,"stated_source":20000}');
