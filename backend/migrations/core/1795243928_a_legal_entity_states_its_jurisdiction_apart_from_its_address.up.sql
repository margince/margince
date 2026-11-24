-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- A further legal entity names the jurisdiction pack whose law it follows,
-- apart from the country of its address. The default entity keeps none: its
-- law is installation.country. The contacts store requires one on a further
-- entity; a row with none takes the installation's law.
SET LOCAL lock_timeout = '3s';

ALTER TABLE legal_entity ADD COLUMN IF NOT EXISTS jurisdiction text;
ALTER TABLE legal_entity DROP CONSTRAINT IF EXISTS legal_entity_jurisdiction_shape;
ALTER TABLE legal_entity ADD CONSTRAINT legal_entity_jurisdiction_shape
    CHECK (jurisdiction IS NULL OR jurisdiction ~ '^[a-z]{2}$') NOT VALID;

-- Until now a further entity's law was its address country, so each one keeps
-- that law.
UPDATE legal_entity SET jurisdiction = lower(country)
 WHERE NOT is_default AND country IS NOT NULL AND jurisdiction IS NULL;

CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."legal_entity" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"address_city":20000,"address_line1":500,"address_line2":500,"address_postal_code":20000,"address_region":20000,"bank_account_holder":20000,"bic":20000,"captured_by":20000,"country":20000,"default_language":20000,"iban":20000,"icp":20000,"imprint":20000,"jurisdiction":20000,"legal_name":500,"logo_object_key":20000,"offer_summary":20000,"register_court":20000,"register_number":20000,"signature_template":20000,"usp":20000,"value_proposition":20000,"vat_id":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"address_city":20000,"address_line1":500,"address_line2":500,"address_postal_code":20000,"address_region":20000,"bank_account_holder":20000,"bic":20000,"captured_by":20000,"country":20000,"default_language":20000,"iban":20000,"icp":20000,"imprint":20000,"jurisdiction":20000,"legal_name":500,"logo_object_key":20000,"offer_summary":20000,"register_court":20000,"register_number":20000,"signature_template":20000,"usp":20000,"value_proposition":20000,"vat_id":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."legal_entity" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"address_city":20000,"address_line1":500,"address_line2":500,"address_postal_code":20000,"address_region":20000,"bank_account_holder":20000,"bic":20000,"captured_by":20000,"country":20000,"default_language":20000,"iban":20000,"icp":20000,"imprint":20000,"jurisdiction":20000,"legal_name":500,"logo_object_key":20000,"offer_summary":20000,"register_court":20000,"register_number":20000,"signature_template":20000,"usp":20000,"value_proposition":20000,"vat_id":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"address_city":20000,"address_line1":500,"address_line2":500,"address_postal_code":20000,"address_region":20000,"bank_account_holder":20000,"bic":20000,"captured_by":20000,"country":20000,"default_language":20000,"iban":20000,"icp":20000,"imprint":20000,"jurisdiction":20000,"legal_name":500,"logo_object_key":20000,"offer_summary":20000,"register_court":20000,"register_number":20000,"signature_template":20000,"usp":20000,"value_proposition":20000,"vat_id":20000}');
