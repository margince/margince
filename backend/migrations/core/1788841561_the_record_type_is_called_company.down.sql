-- Undo of the company rename: every statement in the up file, inverted.
--
-- The names go back FIRST and the function bodies last. A function body
-- naming a column is parsed when the function is created, so recreating one
-- before its column is renamed back fails on the spot -- which is how this
-- file was found to be in the wrong order.
SET LOCAL lock_timeout = '3s';

-- 11. the open vocabularies, back
UPDATE role
   SET permissions = (permissions - 'company')
                     || jsonb_build_object('organization', permissions -> 'company')
 WHERE permissions ? 'company';

UPDATE field_mask SET object = 'organization' WHERE object = 'company';

UPDATE approval SET kind = 'org_name_promotion' WHERE kind = 'company_name_promotion';
UPDATE approval_autonomy_policy SET kind = 'org_name_promotion' WHERE kind = 'company_name_promotion';

UPDATE webhook_subscription
   SET event_types = (
         SELECT array_agg(replace(t, 'company.', 'organization.') ORDER BY t)
           FROM unnest(event_types) AS t
       )
 WHERE EXISTS (SELECT 1 FROM unnest(event_types) AS t WHERE t LIKE 'company.%');

-- 10. the values, the predicate and the two sentences, back.
--
-- First here, last in the up file: it names the renamed index and column, which
-- exist under those names only before section 1 puts them back.

ALTER TABLE site_read ALTER COLUMN target_kind SET DEFAULT 'organization';

DROP INDEX uq_site_read_company_inflight;
CREATE UNIQUE INDEX uq_site_read_company_inflight ON site_read (company_id, seed_url)
  WHERE target_kind = 'organization' AND status IN ('queued', 'deferred', 'running');

COMMENT ON COLUMN deal.partner_attribution IS
  'What the partner named by partner_org_id did for this deal: sourced (they brought it) or influenced (they helped one we already had). Commission accrues on sourced only.';

-- One thing this does not put back: the comment below loses the retired
-- specification number the baseline wrote beside the decision record. A new
-- file may not carry one — the document it names no longer exists, so a
-- reader meeting it has no move — and the surviving record still labels it.
COMMENT ON COLUMN company.classification IS
  'RETIRED (ADR-0079) — superseded by organization.lifecycle + organization_relationship_type. Written by nothing; dropped in a follow-up migration.';

-- 1. tables
ALTER TABLE company_brief RENAME TO org_brief;
ALTER TABLE company_dossier RENAME TO org_dossier;
ALTER TABLE company_growth_fit RENAME TO org_growth_fit;
ALTER TABLE company_scan RENAME TO org_scan;
ALTER TABLE company RENAME TO organization;
ALTER TABLE company_domain RENAME TO organization_domain;
ALTER TABLE company_domain_disposition RENAME TO organization_domain_disposition;
ALTER TABLE company_fact RENAME TO organization_fact;
ALTER TABLE company_geocode_state RENAME TO organization_geocode_state;
ALTER TABLE company_profile_field RENAME TO organization_profile_field;
ALTER TABLE company_relationship_type RENAME TO organization_relationship_type;
ALTER TABLE company_technical_state RENAME TO organization_technical_state;
ALTER TABLE company_vat_check RENAME TO organization_vat_check;

-- 2. columns
ALTER TABLE activity_link RENAME COLUMN company_id TO organization_id;
ALTER TABLE attachment RENAME COLUMN company_id TO organization_id;
ALTER TABLE capture_auto_enrich_state RENAME COLUMN company_id TO organization_id;
ALTER TABLE capture_backfill RENAME COLUMN companies_created TO organizations_created;
ALTER TABLE commission_entry RENAME COLUMN partner_company_id TO partner_org_id;
ALTER TABLE contract RENAME COLUMN company_id TO organization_id;
ALTER TABLE deal RENAME COLUMN company_id TO organization_id;
ALTER TABLE deal RENAME COLUMN partner_company_id TO partner_org_id;
ALTER TABLE dedupe_candidate RENAME COLUMN left_company_id TO left_org_id;
ALTER TABLE dedupe_candidate RENAME COLUMN right_company_id TO right_org_id;
ALTER TABLE finance_customer_link RENAME COLUMN company_id TO organization_id;
ALTER TABLE finance_invoice RENAME COLUMN company_id TO organization_id;
ALTER TABLE finance_payment RENAME COLUMN company_id TO organization_id;
ALTER TABLE lead RENAME COLUMN candidate_company_key TO candidate_org_key;
ALTER TABLE linkedin_connection RENAME COLUMN matched_company_id TO matched_org_id;
ALTER TABLE offer RENAME COLUMN buyer_company_id TO buyer_org_id;
ALTER TABLE org_brief RENAME COLUMN company_id TO organization_id;
ALTER TABLE org_dossier RENAME COLUMN company_id TO organization_id;
ALTER TABLE org_growth_fit RENAME COLUMN company_id TO organization_id;
ALTER TABLE org_scan RENAME COLUMN company_id TO organization_id;
ALTER TABLE organization RENAME COLUMN parent_company_id TO parent_org_id;
ALTER TABLE organization_domain RENAME COLUMN company_id TO organization_id;
ALTER TABLE organization_domain_disposition RENAME COLUMN company_id TO organization_id;
ALTER TABLE organization_fact RENAME COLUMN company_id TO organization_id;
ALTER TABLE organization_geocode_state RENAME COLUMN company_id TO organization_id;
ALTER TABLE organization_profile_field RENAME COLUMN company_id TO organization_id;
ALTER TABLE organization_relationship_type RENAME COLUMN company_id TO organization_id;
ALTER TABLE organization_technical_state RENAME COLUMN company_id TO organization_id;
ALTER TABLE organization_vat_check RENAME COLUMN company_id TO organization_id;
ALTER TABLE partner RENAME COLUMN company_id TO organization_id;
ALTER TABLE project RENAME COLUMN company_id TO organization_id;
ALTER TABLE relationship RENAME COLUMN counterparty_company_id TO counterparty_org_id;
ALTER TABLE relationship RENAME COLUMN company_id TO organization_id;
ALTER TABLE sdr_handoff RENAME COLUMN company_id TO organization_id;
ALTER TABLE signal RENAME COLUMN resolved_company_id TO resolved_org_id;
ALTER TABLE signal_resolution RENAME COLUMN matched_company_id TO matched_org_id;
ALTER TABLE signal_thread_scan RENAME COLUMN resolved_company_id TO resolved_org_id;
ALTER TABLE site_read RENAME COLUMN company_id TO organization_id;
ALTER TABLE suggestion_dismissal RENAME COLUMN company_id TO organization_id;

-- 3. constraints
ALTER TABLE activity_link RENAME CONSTRAINT activity_link_company_id_fkey TO activity_link_organization_id_fkey;
ALTER TABLE capture_auto_enrich_state RENAME CONSTRAINT capture_auto_enrich_state_company_id_fkey TO capture_auto_enrich_state_organization_id_fkey;
ALTER TABLE commission_entry RENAME CONSTRAINT commission_entry_partner_company_id_fkey TO commission_entry_partner_org_id_fkey;
ALTER TABLE contract RENAME CONSTRAINT contract_company_id_fkey TO contract_organization_id_fkey;
ALTER TABLE deal RENAME CONSTRAINT deal_company_id_fkey TO deal_organization_id_fkey;
ALTER TABLE deal RENAME CONSTRAINT deal_partner_company_id_fkey TO deal_partner_org_id_fkey;
ALTER TABLE dedupe_candidate RENAME CONSTRAINT dedupe_candidate_left_company_id_fkey TO dedupe_candidate_left_org_id_fkey;
ALTER TABLE dedupe_candidate RENAME CONSTRAINT dedupe_candidate_right_company_id_fkey TO dedupe_candidate_right_org_id_fkey;
ALTER TABLE finance_customer_link RENAME CONSTRAINT finance_customer_link_company_fk TO finance_customer_link_organization_fk;
ALTER TABLE finance_invoice RENAME CONSTRAINT finance_invoice_company_fk TO finance_invoice_organization_fk;
ALTER TABLE finance_payment RENAME CONSTRAINT finance_payment_company_fk TO finance_payment_organization_fk;
ALTER TABLE linkedin_connection RENAME CONSTRAINT linkedin_connection_workspace_id_matched_company_id_fkey TO linkedin_connection_workspace_id_matched_org_id_fkey;
ALTER TABLE offer RENAME CONSTRAINT offer_buyer_company_fkey TO offer_buyer_org_fkey;
ALTER TABLE org_brief RENAME CONSTRAINT company_brief_generated_by_check TO org_brief_generated_by_check;
ALTER TABLE org_brief RENAME CONSTRAINT company_brief_company_fkey TO org_brief_org_fkey;
ALTER TABLE org_brief RENAME CONSTRAINT company_brief_pkey TO org_brief_pkey;
ALTER TABLE org_brief RENAME CONSTRAINT company_brief_user_id_fkey TO org_brief_user_id_fkey;
ALTER TABLE org_brief RENAME CONSTRAINT company_brief_user_id_company_id_key TO org_brief_user_id_organization_id_key;
ALTER TABLE org_dossier RENAME CONSTRAINT company_dossier_generated_by_check TO org_dossier_generated_by_check;
ALTER TABLE org_dossier RENAME CONSTRAINT company_dossier_company_fkey TO org_dossier_org_fkey;
ALTER TABLE org_dossier RENAME CONSTRAINT company_dossier_pkey TO org_dossier_pkey;
ALTER TABLE org_dossier RENAME CONSTRAINT company_dossier_user_fkey TO org_dossier_user_fkey;
ALTER TABLE org_growth_fit RENAME CONSTRAINT company_growth_fit_generated_by_check TO org_growth_fit_generated_by_check;
ALTER TABLE org_growth_fit RENAME CONSTRAINT company_growth_fit_company_fkey TO org_growth_fit_org_fkey;
ALTER TABLE org_growth_fit RENAME CONSTRAINT company_growth_fit_pkey TO org_growth_fit_pkey;
ALTER TABLE org_growth_fit RENAME CONSTRAINT company_growth_fit_user_fkey TO org_growth_fit_user_fkey;
ALTER TABLE org_scan RENAME CONSTRAINT company_scan_attempt_check TO org_scan_attempt_check;
ALTER TABLE org_scan RENAME CONSTRAINT company_scan_generated_by_check TO org_scan_generated_by_check;
ALTER TABLE org_scan RENAME CONSTRAINT company_scan_company_fkey TO org_scan_org_fkey;
ALTER TABLE org_scan RENAME CONSTRAINT company_scan_pkey TO org_scan_pkey;
ALTER TABLE org_scan RENAME CONSTRAINT company_scan_read_deals_check TO org_scan_read_deals_check;
ALTER TABLE org_scan RENAME CONSTRAINT company_scan_read_exchanges_check TO org_scan_read_exchanges_check;
ALTER TABLE org_scan RENAME CONSTRAINT company_scan_running_has_started TO org_scan_running_has_started;
ALTER TABLE org_scan RENAME CONSTRAINT company_scan_settled_has_finish TO org_scan_settled_has_finish;
ALTER TABLE org_scan RENAME CONSTRAINT company_scan_status_check TO org_scan_status_check;
ALTER TABLE org_scan RENAME CONSTRAINT company_scan_user_id_fkey TO org_scan_user_id_fkey;
ALTER TABLE org_scan RENAME CONSTRAINT company_scan_user_id_company_id_key TO org_scan_user_id_organization_id_key;
ALTER TABLE organization RENAME CONSTRAINT company_anchor_is_permanent TO organization_anchor_is_permanent;
ALTER TABLE organization RENAME CONSTRAINT company_classification_check TO organization_classification_check;
ALTER TABLE organization RENAME CONSTRAINT company_description_length TO organization_description_length;
ALTER TABLE organization RENAME CONSTRAINT company_geocode_resolved_has_a_point TO organization_geocode_resolved_has_a_point;
ALTER TABLE organization RENAME CONSTRAINT company_geocode_status_check TO organization_geocode_status_check;
ALTER TABLE organization RENAME CONSTRAINT company_lifecycle_check TO organization_lifecycle_check;
ALTER TABLE organization RENAME CONSTRAINT company_linkedin_url_shape TO organization_linkedin_url_shape;
ALTER TABLE organization RENAME CONSTRAINT company_merged_into_id_fkey TO organization_merged_into_id_fkey;
ALTER TABLE organization RENAME CONSTRAINT company_name_source_check TO organization_name_source_check;
ALTER TABLE organization RENAME CONSTRAINT company_not_own_parent TO organization_not_own_parent;
ALTER TABLE organization RENAME CONSTRAINT company_owner_id_fkey TO organization_owner_id_fkey;
ALTER TABLE organization RENAME CONSTRAINT company_owner_private_names_its_owner TO organization_owner_private_names_its_owner;
ALTER TABLE organization RENAME CONSTRAINT company_parent_company_id_fkey TO organization_parent_org_id_fkey;
ALTER TABLE organization RENAME CONSTRAINT company_pkey TO organization_pkey;
ALTER TABLE organization RENAME CONSTRAINT company_relevance_check TO organization_relevance_check;
ALTER TABLE organization RENAME CONSTRAINT company_size_band_check TO organization_size_band_check;
ALTER TABLE organization RENAME CONSTRAINT company_visibility_check TO organization_visibility_check;
ALTER TABLE organization_domain RENAME CONSTRAINT company_domain_norm TO org_domain_norm;
ALTER TABLE organization_domain RENAME CONSTRAINT company_domain_company_id_fkey TO organization_domain_organization_id_fkey;
ALTER TABLE organization_domain RENAME CONSTRAINT company_domain_pkey TO organization_domain_pkey;
ALTER TABLE organization_domain_disposition RENAME CONSTRAINT company_domain_disposition_admission_check TO organization_domain_disposition_admission_check;
ALTER TABLE organization_domain_disposition RENAME CONSTRAINT company_domain_disposition_admission_shape TO organization_domain_disposition_admission_shape;
ALTER TABLE organization_domain_disposition RENAME CONSTRAINT company_domain_disposition_admission_source_check TO organization_domain_disposition_admission_source_check;
ALTER TABLE organization_domain_disposition RENAME CONSTRAINT company_domain_disposition_domain_check TO organization_domain_disposition_domain_check;
ALTER TABLE organization_domain_disposition RENAME CONSTRAINT company_domain_disposition_company_fkey TO organization_domain_disposition_org_fkey;
ALTER TABLE organization_domain_disposition RENAME CONSTRAINT company_domain_disposition_owner_fkey TO organization_domain_disposition_owner_fkey;
ALTER TABLE organization_domain_disposition RENAME CONSTRAINT company_domain_disposition_pending_reason_check TO organization_domain_disposition_pending_reason_check;
ALTER TABLE organization_domain_disposition RENAME CONSTRAINT company_domain_disposition_pending_reason_shape TO organization_domain_disposition_pending_reason_shape;
ALTER TABLE organization_domain_disposition RENAME CONSTRAINT company_domain_disposition_pkey TO organization_domain_disposition_pkey;
ALTER TABLE organization_domain_disposition RENAME CONSTRAINT company_domain_disposition_settled_shape TO organization_domain_disposition_settled_shape;
ALTER TABLE organization_domain_disposition RENAME CONSTRAINT company_domain_disposition_site_read_fkey TO organization_domain_disposition_site_read_fkey;
ALTER TABLE organization_domain_disposition RENAME CONSTRAINT company_domain_disposition_source_check TO organization_domain_disposition_source_check;
ALTER TABLE organization_domain_disposition RENAME CONSTRAINT company_domain_disposition_status_check TO organization_domain_disposition_status_check;
ALTER TABLE organization_fact RENAME CONSTRAINT company_fact_field_vocab TO org_fact_field_vocab;
ALTER TABLE organization_fact RENAME CONSTRAINT company_fact_company_fkey TO org_fact_org_fkey;
ALTER TABLE organization_fact RENAME CONSTRAINT company_fact_site_evidence TO org_fact_site_evidence;
ALTER TABLE organization_fact RENAME CONSTRAINT company_fact_site_read_fkey TO org_fact_site_read_fkey;
ALTER TABLE organization_fact RENAME CONSTRAINT company_fact_technical_evidence TO org_fact_technical_evidence;
ALTER TABLE organization_fact RENAME CONSTRAINT company_fact_value_key_cardinality TO org_fact_value_key_cardinality;
ALTER TABLE organization_fact RENAME CONSTRAINT company_fact_verified_pair TO org_fact_verified_pair;
ALTER TABLE organization_fact RENAME CONSTRAINT company_fact_category_check TO organization_fact_category_check;
ALTER TABLE organization_fact RENAME CONSTRAINT company_fact_confidence_check TO organization_fact_confidence_check;
ALTER TABLE organization_fact RENAME CONSTRAINT company_fact_pkey TO organization_fact_pkey;
ALTER TABLE organization_fact RENAME CONSTRAINT company_fact_source_check TO organization_fact_source_check;
ALTER TABLE organization_fact RENAME CONSTRAINT uq_company_fact TO uq_org_fact;
ALTER TABLE organization_geocode_state RENAME CONSTRAINT company_geocode_state_company_id_fkey TO organization_geocode_state_organization_id_fkey;
ALTER TABLE organization_geocode_state RENAME CONSTRAINT company_geocode_state_pkey TO organization_geocode_state_pkey;
ALTER TABLE organization_profile_field RENAME CONSTRAINT company_profile_field_company_fkey TO org_profile_field_org_fkey;
ALTER TABLE organization_profile_field RENAME CONSTRAINT company_profile_field_verified_pair TO org_profile_field_verified_pair;
ALTER TABLE organization_profile_field RENAME CONSTRAINT company_profile_site_evidence TO org_profile_site_evidence;
ALTER TABLE organization_profile_field RENAME CONSTRAINT company_profile_field_confidence_check TO organization_profile_field_confidence_check;
ALTER TABLE organization_profile_field RENAME CONSTRAINT company_profile_field_field_check TO organization_profile_field_field_check;
ALTER TABLE organization_profile_field RENAME CONSTRAINT company_profile_field_pkey TO organization_profile_field_pkey;
ALTER TABLE organization_profile_field RENAME CONSTRAINT company_profile_field_source_check TO organization_profile_field_source_check;
ALTER TABLE organization_profile_field RENAME CONSTRAINT uq_company_profile_field TO uq_org_profile_field;
ALTER TABLE organization_relationship_type RENAME CONSTRAINT company_relationship_typ_workspace_id_company_id_fkey TO organization_relationship_typ_workspace_id_organization_id_fkey;
ALTER TABLE organization_relationship_type RENAME CONSTRAINT company_relationship_type_pkey TO organization_relationship_type_pkey;
ALTER TABLE organization_relationship_type RENAME CONSTRAINT company_relationship_type_relationship_type_check TO organization_relationship_type_relationship_type_check;
ALTER TABLE organization_technical_state RENAME CONSTRAINT company_technical_state_lane_check TO organization_technical_state_lane_check;
ALTER TABLE organization_technical_state RENAME CONSTRAINT company_technical_state_company_id_fkey TO organization_technical_state_organization_id_fkey;
ALTER TABLE organization_technical_state RENAME CONSTRAINT company_technical_state_outcome_check TO organization_technical_state_outcome_check;
ALTER TABLE organization_technical_state RENAME CONSTRAINT company_technical_state_pkey TO organization_technical_state_pkey;
ALTER TABLE organization_vat_check RENAME CONSTRAINT company_vat_check_number_not_blank TO organization_vat_check_number_not_blank;
ALTER TABLE organization_vat_check RENAME CONSTRAINT company_vat_check_one_per_company TO organization_vat_check_one_per_org;
ALTER TABLE organization_vat_check RENAME CONSTRAINT company_vat_check_company_fk TO organization_vat_check_org_fk;
ALTER TABLE organization_vat_check RENAME CONSTRAINT company_vat_check_pkey TO organization_vat_check_pkey;
ALTER TABLE organization_vat_check RENAME CONSTRAINT company_vat_check_receipt_needs_an_answer TO organization_vat_check_receipt_needs_an_answer;
ALTER TABLE organization_vat_check RENAME CONSTRAINT company_vat_check_status_check TO organization_vat_check_status_check;
ALTER TABLE partner RENAME CONSTRAINT partner_company_id_fkey TO partner_organization_id_fkey;
ALTER TABLE partner RENAME CONSTRAINT partner_company_id_key TO partner_organization_id_key;
ALTER TABLE project RENAME CONSTRAINT project_company_id_fkey TO project_organization_id_fkey;
ALTER TABLE relationship RENAME CONSTRAINT relationship_counterparty_company_id_fkey TO relationship_counterparty_org_id_fkey;
ALTER TABLE relationship RENAME CONSTRAINT relationship_company_id_fkey TO relationship_organization_id_fkey;
ALTER TABLE sdr_handoff RENAME CONSTRAINT sdr_handoff_company_id_fkey TO sdr_handoff_organization_id_fkey;
ALTER TABLE signal RENAME CONSTRAINT signal_resolved_company_fkey TO signal_resolved_org_fkey;
ALTER TABLE signal_resolution RENAME CONSTRAINT sigres_company_fkey TO sigres_org_fkey;
ALTER TABLE signal_thread_scan RENAME CONSTRAINT signal_thread_scan_resolved_company_fkey TO signal_thread_scan_resolved_org_fkey;
ALTER TABLE site_read RENAME CONSTRAINT site_read_company_fkey TO site_read_org_fkey;
ALTER TABLE suggestion_dismissal RENAME CONSTRAINT suggestion_dismissal_company_fkey TO suggestion_dismissal_org_fkey;
ALTER TABLE suggestion_dismissal RENAME CONSTRAINT suggestion_dismissal_user_id_company_id_f_key TO suggestion_dismissal_user_id_organization_id_f_key;

-- 4. indexes
ALTER INDEX finance_customer_link_company_ux RENAME TO finance_customer_link_organization_ux;
ALTER INDEX idx_alink_company RENAME TO idx_alink_org;
ALTER INDEX idx_deal_company RENAME TO idx_deal_org;
ALTER INDEX idx_lead_cand_company RENAME TO idx_lead_cand_org;
ALTER INDEX idx_linkedin_connection_company RENAME TO idx_linkedin_connection_org;
ALTER INDEX idx_company_class RENAME TO idx_org_class;
ALTER INDEX idx_company_created_keyset RENAME TO idx_org_created_keyset;
ALTER INDEX idx_company_domain_company RENAME TO idx_org_domain_org;
ALTER INDEX idx_company_fact_lookup RENAME TO idx_org_fact_lookup;
ALTER INDEX idx_company_last_activity_keyset RENAME TO idx_org_last_activity_keyset;
ALTER INDEX idx_company_legal_name_trgm RENAME TO idx_org_legal_name_trgm;
ALTER INDEX idx_company_lifecycle RENAME TO idx_org_lifecycle;
ALTER INDEX idx_company_name_keyset RENAME TO idx_org_name_keyset;
ALTER INDEX idx_company_name_keyset_desc RENAME TO idx_org_name_keyset_desc;
ALTER INDEX idx_company_name_trgm RENAME TO idx_org_name_trgm;
ALTER INDEX idx_company_owner RENAME TO idx_org_owner;
ALTER INDEX idx_company_parent RENAME TO idx_org_parent;
ALTER INDEX idx_company_rel_type_cascade RENAME TO idx_org_rel_type_cascade;
ALTER INDEX idx_company_rel_type_company RENAME TO idx_org_rel_type_org;
ALTER INDEX idx_company_search RENAME TO idx_org_search;
ALTER INDEX idx_company_technical_state_due RENAME TO idx_org_technical_state_due;
ALTER INDEX idx_company_updated_keyset RENAME TO idx_org_updated_keyset;
ALTER INDEX idx_company_domain_disposition_due RENAME TO idx_organization_domain_disposition_due;
ALTER INDEX idx_company_geocoded RENAME TO idx_organization_geocoded;
ALTER INDEX idx_project_company RENAME TO idx_project_org;
ALTER INDEX idx_project_company_open RENAME TO idx_project_org_open;
ALTER INDEX idx_rel_history_company RENAME TO idx_rel_history_organization;
ALTER INDEX idx_rel_company_people RENAME TO idx_rel_org_people;
ALTER INDEX idx_rel_partner_company RENAME TO idx_rel_partner_org;
ALTER INDEX idx_rel_person_companies RENAME TO idx_rel_person_orgs;
ALTER INDEX idx_rel_traverse_company RENAME TO idx_rel_traverse_organization;
ALTER INDEX idx_site_read_company RENAME TO idx_site_read_org;
ALTER INDEX company_brief_company_ix RENAME TO org_brief_organization_ix;
ALTER INDEX company_dossier_company_ix RENAME TO org_dossier_organization_ix;
ALTER INDEX company_growth_fit_company_ix RENAME TO org_growth_fit_organization_ix;
ALTER INDEX company_scan_company_ix RENAME TO org_scan_organization_ix;
ALTER INDEX company_linkedin_url_key RENAME TO organization_linkedin_url_key;
ALTER INDEX signal_resolved_company_ix RENAME TO signal_resolved_org_ix;
ALTER INDEX suggestion_dismissal_company_ix RENAME TO suggestion_dismissal_organization_ix;
ALTER INDEX uq_company_domain RENAME TO uq_org_domain;
ALTER INDEX uq_company_domain_primary RENAME TO uq_org_domain_primary;
ALTER INDEX uq_company_rel_type RENAME TO uq_org_rel_type;
ALTER INDEX uq_company_anchor RENAME TO uq_organization_anchor;
ALTER INDEX uq_company_domain_disposition RENAME TO uq_organization_domain_disposition;
ALTER INDEX uq_site_read_company_inflight RENAME TO uq_site_read_org_inflight;

-- 5. triggers
ALTER TABLE deal RENAME CONSTRAINT trg_deal_project_same_company TO trg_deal_project_same_org;
ALTER TRIGGER trg_deal_project_same_company ON deal RENAME TO trg_deal_project_same_org;
ALTER TRIGGER company_delete_clears_deal_partner ON organization RENAME TO organization_delete_clears_deal_partner;
ALTER TRIGGER company_refuse_anchor_retirement ON organization RENAME TO organization_refuse_anchor_retirement;
ALTER TRIGGER trg_company_geocode_stale ON organization RENAME TO trg_organization_geocode_stale;
ALTER TRIGGER trg_company_no_cycle ON organization RENAME TO trg_organization_no_cycle;
ALTER TRIGGER trg_company_updated ON organization RENAME TO trg_organization_updated;
ALTER TRIGGER trg_company_domain_updated ON organization_domain RENAME TO trg_organization_domain_updated;
ALTER TRIGGER trg_company_fact_updated ON organization_fact RENAME TO trg_organization_fact_updated;
ALTER TRIGGER trg_company_profile_field_updated ON organization_profile_field RENAME TO trg_organization_profile_field_updated;
ALTER TRIGGER trg_company_relationship_type_updated ON organization_relationship_type RENAME TO trg_organization_relationship_type_updated;

-- 6. function names
ALTER FUNCTION assert_deal_project_same_company() RENAME TO assert_deal_project_same_org;
ALTER FUNCTION deal_clear_partner_attribution_on_company_delete() RENAME TO deal_clear_partner_attribution_on_org_delete;
ALTER FUNCTION company_geocode_goes_stale() RENAME TO organization_geocode_goes_stale;
ALTER FUNCTION company_no_ancestor_cycle() RENAME TO organization_no_ancestor_cycle;
ALTER FUNCTION company_refuse_anchor_retirement() RENAME TO organization_refuse_anchor_retirement;

-- 7. the stored word, back

-- activity_link.entity_type
ALTER TABLE activity_link DROP CONSTRAINT activity_link_entity_type_check;
ALTER TABLE activity_link DROP CONSTRAINT activity_link_shape;
UPDATE activity_link SET entity_type = 'organization' WHERE entity_type = 'company';
ALTER TABLE activity_link ADD CONSTRAINT activity_link_entity_type_check
  CHECK (entity_type IN ('person', 'organization', 'deal', 'lead', 'project'));
ALTER TABLE activity_link ADD CONSTRAINT activity_link_shape
  CHECK (((entity_type = 'person') AND (person_id IS NOT NULL) AND (organization_id IS NULL) AND (deal_id IS NULL) AND (lead_id IS NULL) AND (project_id IS NULL)) OR ((entity_type = 'organization') AND (organization_id IS NOT NULL) AND (person_id IS NULL) AND (deal_id IS NULL) AND (lead_id IS NULL) AND (project_id IS NULL)) OR ((entity_type = 'deal') AND (deal_id IS NOT NULL) AND (person_id IS NULL) AND (organization_id IS NULL) AND (lead_id IS NULL) AND (project_id IS NULL)) OR ((entity_type = 'lead') AND (lead_id IS NOT NULL) AND (person_id IS NULL) AND (organization_id IS NULL) AND (deal_id IS NULL) AND (project_id IS NULL)) OR ((entity_type = 'project') AND (project_id IS NOT NULL) AND (person_id IS NULL) AND (organization_id IS NULL) AND (deal_id IS NULL) AND (lead_id IS NULL)));

-- ai_feedback.subject_type
ALTER TABLE ai_feedback DROP CONSTRAINT ai_feedback_subject_type_check;
UPDATE ai_feedback SET subject_type = 'organization' WHERE subject_type = 'company';
ALTER TABLE ai_feedback ADD CONSTRAINT ai_feedback_subject_type_check
  CHECK (subject_type IN ('organization', 'person', 'deal', 'lead'));

-- attachment.entity_type
ALTER TABLE attachment DROP CONSTRAINT attachment_entity_type_check;
UPDATE attachment SET entity_type = 'organization' WHERE entity_type = 'company';
ALTER TABLE attachment ADD CONSTRAINT attachment_entity_type_check
  CHECK (entity_type IN ('person', 'organization', 'deal', 'lead', 'activity', 'project', 'relationship', 'partner'));

-- capture_backfill_creation.kind
ALTER TABLE capture_backfill_creation DROP CONSTRAINT capture_backfill_creation_kind;
UPDATE capture_backfill_creation SET kind = 'organization_queued' WHERE kind = 'company_queued';
ALTER TABLE capture_backfill_creation ADD CONSTRAINT capture_backfill_creation_kind
  CHECK (kind IN ('person', 'organization_queued'));

-- capture_pending_counterparty.kind
ALTER TABLE capture_pending_counterparty DROP CONSTRAINT capture_pending_counterparty_kind_check;
UPDATE capture_pending_counterparty SET kind = 'organization_sender' WHERE kind = 'company_sender';
ALTER TABLE capture_pending_counterparty ADD CONSTRAINT capture_pending_counterparty_kind_check
  CHECK ((kind IS NULL) OR kind IN ('person', 'role_mailbox', 'organization_sender', 'newsletter', 'transactional', 'spam', 'personal', 'advisor'));

-- custom_field.object
ALTER TABLE custom_field DROP CONSTRAINT custom_field_object_check;
UPDATE custom_field SET object = 'organization' WHERE object = 'company';
ALTER TABLE custom_field ADD CONSTRAINT custom_field_object_check
  CHECK (object IN ('person', 'organization', 'deal', 'lead', 'activity', 'project', 'relationship', 'partner'));

-- dedupe_candidate.entity_type
ALTER TABLE dedupe_candidate DROP CONSTRAINT dedupe_candidate_entity_type_check;
ALTER TABLE dedupe_candidate DROP CONSTRAINT dedupe_candidate_shape;
UPDATE dedupe_candidate SET entity_type = 'organization' WHERE entity_type = 'company';
ALTER TABLE dedupe_candidate ADD CONSTRAINT dedupe_candidate_entity_type_check
  CHECK (entity_type IN ('person', 'organization', 'lead'));
ALTER TABLE dedupe_candidate ADD CONSTRAINT dedupe_candidate_shape
  CHECK (((entity_type = 'person') AND (left_person_id IS NOT NULL) AND (right_person_id IS NOT NULL) AND (left_org_id IS NULL) AND (right_org_id IS NULL) AND (left_lead_id IS NULL) AND (right_lead_id IS NULL)) OR ((entity_type = 'organization') AND (left_org_id IS NOT NULL) AND (right_org_id IS NOT NULL) AND (left_person_id IS NULL) AND (right_person_id IS NULL) AND (left_lead_id IS NULL) AND (right_lead_id IS NULL)) OR ((entity_type = 'lead') AND (left_lead_id IS NOT NULL) AND (right_lead_id IS NOT NULL) AND (left_person_id IS NULL) AND (right_person_id IS NULL) AND (left_org_id IS NULL) AND (right_org_id IS NULL)));

-- embedding.entity_type
ALTER TABLE embedding DROP CONSTRAINT embedding_entity_type_check;
UPDATE embedding SET entity_type = 'organization' WHERE entity_type = 'company';
ALTER TABLE embedding ADD CONSTRAINT embedding_entity_type_check
  CHECK (entity_type IN ('person', 'organization', 'deal', 'lead', 'activity', 'project', 'relationship', 'partner'));

-- field_provenance.object_type
ALTER TABLE field_provenance DROP CONSTRAINT field_provenance_object_type_check;
UPDATE field_provenance SET object_type = 'organization' WHERE object_type = 'company';
ALTER TABLE field_provenance ADD CONSTRAINT field_provenance_object_type_check
  CHECK (object_type IN ('person', 'organization', 'deal', 'lead', 'activity', 'project', 'relationship', 'partner'));

-- list.entity_type
ALTER TABLE list DROP CONSTRAINT list_entity_type_check;
UPDATE list SET entity_type = 'organization' WHERE entity_type = 'company';
ALTER TABLE list ADD CONSTRAINT list_entity_type_check
  CHECK (entity_type IN ('person', 'organization', 'deal', 'lead', 'project'));

-- list_member.entity_type
ALTER TABLE list_member DROP CONSTRAINT list_member_entity_type_check;
UPDATE list_member SET entity_type = 'organization' WHERE entity_type = 'company';
ALTER TABLE list_member ADD CONSTRAINT list_member_entity_type_check
  CHECK (entity_type IN ('person', 'organization', 'deal', 'lead', 'project'));

-- person_profile_field.field
ALTER TABLE person_profile_field DROP CONSTRAINT person_profile_field_field_check;
UPDATE person_profile_field SET field = 'org_name' WHERE field = 'company_name';
ALTER TABLE person_profile_field ADD CONSTRAINT person_profile_field_field_check
  CHECK (field IN ('title', 'phone', 'role', 'linkedin', 'org_name', 'address', 'website'));

-- record_grant.record_type
ALTER TABLE record_grant DROP CONSTRAINT record_grant_record_type_check;
UPDATE record_grant SET record_type = 'organization' WHERE record_type = 'company';
ALTER TABLE record_grant ADD CONSTRAINT record_grant_record_type_check
  CHECK (record_type IN ('person', 'organization', 'deal', 'lead', 'project'));

-- saved_view.resource
ALTER TABLE saved_view DROP CONSTRAINT saved_view_resource_check;
UPDATE saved_view SET resource = 'organizations' WHERE resource = 'companies';
ALTER TABLE saved_view ADD CONSTRAINT saved_view_resource_check
  CHECK (resource IN ('people', 'organizations', 'deals', 'activities', 'leads', 'partners', 'projects'));

-- signal.entity_type
ALTER TABLE signal DROP CONSTRAINT signal_entity_type_check;
UPDATE signal SET entity_type = 'organization' WHERE entity_type = 'company';
ALTER TABLE signal ADD CONSTRAINT signal_entity_type_check
  CHECK ((entity_type IS NULL) OR entity_type IN ('deal', 'organization', 'person', 'project'));

-- site_read.target_kind
ALTER TABLE site_read DROP CONSTRAINT site_read_target_kind_check;
ALTER TABLE site_read DROP CONSTRAINT site_read_target_shape;
UPDATE site_read SET target_kind = 'organization' WHERE target_kind = 'company';
ALTER TABLE site_read ADD CONSTRAINT site_read_target_kind_check
  CHECK (target_kind IN ('onboarding', 'organization', 'domain_triage'));
ALTER TABLE site_read ADD CONSTRAINT site_read_target_shape
  CHECK (((target_kind = 'onboarding') AND ((organization_id IS NULL) OR ((organization_id IS NOT NULL) AND (confirmed_at IS NOT NULL)))) OR ((target_kind = 'organization') AND (organization_id IS NOT NULL)) OR ((target_kind = 'domain_triage') AND ((organization_id IS NULL) OR ((organization_id IS NOT NULL) AND (confirmed_at IS NOT NULL)))));

-- taggable.entity_type
ALTER TABLE taggable DROP CONSTRAINT taggable_entity_type_check;
UPDATE taggable SET entity_type = 'organization' WHERE entity_type = 'company';
ALTER TABLE taggable ADD CONSTRAINT taggable_entity_type_check
  CHECK (entity_type IN ('person', 'organization', 'deal', 'lead', 'project'));

-- user_record_view.entity_type
ALTER TABLE user_record_view DROP CONSTRAINT user_record_view_entity_type_check;
UPDATE user_record_view SET entity_type = 'organization' WHERE entity_type = 'company';
ALTER TABLE user_record_view ADD CONSTRAINT user_record_view_entity_type_check
  CHECK (entity_type IN ('organization', 'person'));

-- weekly_plan_commitment.linked_record_type
ALTER TABLE weekly_plan_commitment DROP CONSTRAINT weekly_plan_commitment_link_type_check;
UPDATE weekly_plan_commitment SET linked_record_type = 'organization' WHERE linked_record_type = 'company';
ALTER TABLE weekly_plan_commitment ADD CONSTRAINT weekly_plan_commitment_link_type_check
  CHECK ((linked_record_type IS NULL) OR linked_record_type IN ('deal', 'lead', 'person', 'organization', 'project'));

-- 8. function bodies, as they were

CREATE OR REPLACE FUNCTION activity_link_refuses_a_company_meeting()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    activity_kind text;
BEGIN
    IF NEW.entity_type <> 'organization' THEN
        RETURN NEW;
    END IF;

    -- LOCKED BEFORE THE CHECK, and the same row the other trigger locks. Read
    -- unlocked, one transaction can insert this link while the activity is
    -- still a `note` (the check below passes) while another re-kinds that same
    -- activity to `meeting` before the link is visible (the check in
    -- activity_refuses_becoming_a_company_meeting passes too). Both commit, and
    -- the row neither of them was allowed to make exists.
    --
    -- The ORDER is the whole of it: locked first, checked second, and the same
    -- row the re-kind trigger locks. A BEFORE ROW trigger runs ahead of the
    -- UPDATE's own row lock, so a re-kind whose check has already run cannot be
    -- made to look again by blocking it afterwards — it would wake and write on
    -- the answer it took before waiting. Locking first is what makes each
    -- check happen after the other transaction has finished or not started.
    --
    -- SHARE here and UPDATE there, which is the pairing that conflicts where it
    -- should: a link and a re-kind queue, while two links onto one activity do
    -- not — they are not in conflict and have no reason to wait for each other.
    PERFORM 1 FROM activity WHERE id = NEW.activity_id FOR SHARE;
    SELECT kind INTO activity_kind FROM activity WHERE id = NEW.activity_id;

    IF activity_kind IN ('meeting', 'call') THEN
        RAISE EXCEPTION
            'a % is with a person, not with a company: link it to the person who was there, and the company sees it through them',
            activity_kind
            USING ERRCODE = 'check_violation';
    END IF;

    RETURN NEW;
END;
$$;

CREATE OR REPLACE FUNCTION activity_refuses_becoming_a_company_meeting()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    IF NEW.kind NOT IN ('meeting', 'call') OR OLD.kind = NEW.kind THEN
        RETURN NEW;
    END IF;

    -- The other half of the pairing, taken BEFORE the check for the same
    -- reason: this trigger runs ahead of the UPDATE's own row lock, so waiting
    -- afterwards would leave it writing on an answer it read before a
    -- concurrent link landed. FOR UPDATE, so it conflicts with the link side's
    -- SHARE.
    PERFORM 1 FROM activity WHERE id = NEW.id FOR UPDATE;

    IF EXISTS (SELECT 1 FROM activity_link
               WHERE activity_id = NEW.id AND entity_type = 'organization') THEN
        RAISE EXCEPTION
            'this activity is linked to a company, so it cannot become a %: a % is with a person',
            NEW.kind, NEW.kind
            USING ERRCODE = 'check_violation';
    END IF;

    RETURN NEW;
END;
$$;

CREATE OR REPLACE FUNCTION assert_deal_project_same_org() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
  IF NEW.project_id IS NULL THEN
    RETURN NULL;
  END IF;
  IF NOT EXISTS (
    SELECT 1 FROM relationship r
    WHERE r.kind = 'project_company'
      AND r.project_id = NEW.project_id
      AND r.organization_id IS NOT DISTINCT FROM NEW.organization_id
      AND r.archived_at IS NULL
  ) THEN
    RAISE EXCEPTION 'the deal names a company that is not on this project'
      USING ERRCODE = 'check_violation', CONSTRAINT = 'deal_project_same_org';
  END IF;
  RETURN NULL;
END;
$$;

CREATE OR REPLACE FUNCTION deal_clear_partner_attribution_on_org_delete() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
  UPDATE deal
     SET partner_org_id = NULL, partner_attribution = NULL
   WHERE partner_org_id = OLD.id;
  RETURN OLD;
END;
$$;

CREATE OR REPLACE FUNCTION organization_geocode_goes_stale() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
  -- Only when the coordinates exist and the writer did not speak for them:
  -- stamping 'stale' on a row that was never resolved would say the
  -- coordinates are out of date rather than absent, and overriding a writer
  -- that set the status deliberately would undo the worker's own write.
  IF NEW.geocode_status IS DISTINCT FROM OLD.geocode_status THEN
    RETURN NEW;
  END IF;
  IF OLD.geocode_status IS NULL THEN
    RETURN NEW;
  END IF;
  NEW.geocode_status := 'stale';
  RETURN NEW;
END;
$$;

CREATE OR REPLACE FUNCTION organization_no_ancestor_cycle() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
  ancestor uuid := NEW.parent_org_id;
BEGIN
  WHILE ancestor IS NOT NULL LOOP
    IF ancestor = NEW.id THEN
      RAISE EXCEPTION 'organization % would become its own ancestor', NEW.id
        USING ERRCODE = 'check_violation';
    END IF;
    SELECT parent_org_id INTO ancestor FROM organization WHERE id = ancestor;
  END LOOP;
  RETURN NEW;
END;
$$;

CREATE OR REPLACE FUNCTION organization_refuse_anchor_retirement() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
  IF NEW.merged_into_id IS NOT NULL
     AND EXISTS (SELECT 1 FROM organization o
                  WHERE o.id = NEW.merged_into_id AND o.is_anchor) THEN
    RAISE EXCEPTION 'organization % may not be merged into the anchor organization', NEW.id
      USING ERRCODE = 'check_violation',
            CONSTRAINT = 'organization_anchor_is_permanent';
  END IF;
  IF TG_OP = 'UPDATE' AND OLD.is_anchor AND NOT NEW.is_anchor
     AND OLD.archived_at IS NULL AND OLD.merged_into_id IS NULL THEN
    RAISE EXCEPTION 'organization % is the anchor organization and may not be demoted', NEW.id
      USING ERRCODE = 'check_violation',
            CONSTRAINT = 'organization_anchor_is_permanent';
  END IF;
  RETURN NEW;
END;
$$;

CREATE OR REPLACE FUNCTION trg_activity_last_activity() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
  PERFORM refresh_last_activity_for_link(l.person_id, l.deal_id, l.organization_id)
     FROM activity_link l WHERE l.activity_id = NEW.id;
  RETURN NULL;
END;
$$;

CREATE OR REPLACE FUNCTION trg_activity_link_last_activity() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
  IF TG_OP IN ('DELETE', 'UPDATE') THEN
    PERFORM refresh_last_activity_for_link(OLD.person_id, OLD.deal_id, OLD.organization_id);
  END IF;
  IF TG_OP IN ('INSERT', 'UPDATE') THEN
    PERFORM refresh_last_activity_for_link(NEW.person_id, NEW.deal_id, NEW.organization_id);
  END IF;
  RETURN NULL;
END;
$$;

CREATE OR REPLACE FUNCTION trg_deal_last_activity() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
  PERFORM move_last_activity('organization', OLD.organization_id);
  PERFORM move_last_activity('organization', NEW.organization_id);
  RETURN NULL;
END;
$$;

CREATE OR REPLACE FUNCTION trg_relationship_last_activity() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
  IF TG_OP IN ('DELETE', 'UPDATE') AND OLD.kind = 'employment' THEN
    PERFORM move_last_activity('organization', OLD.organization_id);
  END IF;
  IF TG_OP IN ('INSERT', 'UPDATE') AND NEW.kind = 'employment' THEN
    PERFORM move_last_activity('organization', NEW.organization_id);
  END IF;
  RETURN NULL;
END;
$$;

-- 9. the argument-taking functions, as they were

DROP FUNCTION refresh_last_activity_for_link(uuid, uuid, uuid);
DROP FUNCTION last_activity_of_company(uuid);
DROP FUNCTION company_open_pipeline_rollup(date);

CREATE FUNCTION last_activity_of_organization(oid uuid) RETURNS timestamptz
    LANGUAGE sql STABLE
    AS $$
  SELECT max(v) FROM (
    -- Filed against the account itself.
    SELECT max(a.occurred_at) AS v
      FROM activity_link l
      JOIN activity a ON a.id = l.activity_id AND a.archived_at IS NULL
       AND a.audience = 'workspace'
       AND a.origin NOT IN ('system_remediation', 'system_notice')
     WHERE l.organization_id = oid
    UNION ALL
    -- Filed against one of its deals.
    SELECT max(a.occurred_at)
      FROM deal d
      JOIN activity_link l ON l.deal_id = d.id
      JOIN activity a ON a.id = l.activity_id AND a.archived_at IS NULL
       AND a.audience = 'workspace'
       AND a.origin NOT IN ('system_remediation', 'system_notice')
     WHERE d.organization_id = oid
    UNION ALL
    -- Filed against a contact it currently employs.
    SELECT max(a.occurred_at)
      FROM relationship r
      JOIN activity_link l ON l.person_id = r.person_id
      JOIN activity a ON a.id = l.activity_id AND a.archived_at IS NULL
       AND a.audience = 'workspace'
       AND a.origin NOT IN ('system_remediation', 'system_notice')
     WHERE r.organization_id = oid AND r.kind = 'employment'
       AND r.ended_at IS NULL AND r.archived_at IS NULL
  ) arms
$$;;

CREATE FUNCTION refresh_last_activity_for_link(pid uuid, did uuid, oid uuid) RETURNS void
    LANGUAGE plpgsql
    AS $$
DECLARE
  reached uuid;
BEGIN
  PERFORM move_last_activity('person', pid);
  PERFORM move_last_activity('deal', did);
  -- Ordered by id: two writers reaching the same accounts lock them in the
  -- same order, so they queue rather than deadlock.
  FOR reached IN
     SELECT x FROM (
       SELECT oid AS x WHERE oid IS NOT NULL
       UNION SELECT d.organization_id FROM deal d WHERE d.id = did AND d.organization_id IS NOT NULL
       UNION SELECT r.organization_id FROM relationship r
              WHERE r.person_id = pid AND r.kind = 'employment' AND r.ended_at IS NULL AND r.archived_at IS NULL
     ) reach ORDER BY x
  LOOP
    PERFORM move_last_activity('organization', reached);
  END LOOP;
END;
$$;;

CREATE OR REPLACE FUNCTION move_last_activity(tbl regclass, rid uuid) RETURNS void
    LANGUAGE plpgsql
    AS $$
DECLARE
  v timestamptz;
BEGIN
  IF rid IS NULL THEN RETURN; END IF;
  CASE tbl
    WHEN 'person'::regclass THEN
      PERFORM 1 FROM person WHERE id = rid FOR UPDATE;
      v := last_activity_of_person(rid);
      PERFORM set_config('margince.last_activity_move', 'on', true);
      UPDATE person SET last_activity_at = v WHERE id = rid;
    WHEN 'deal'::regclass THEN
      PERFORM 1 FROM deal WHERE id = rid FOR UPDATE;
      v := last_activity_of_deal(rid);
      PERFORM set_config('margince.last_activity_move', 'on', true);
      UPDATE deal SET last_activity_at = v WHERE id = rid;
    WHEN 'organization'::regclass THEN
      PERFORM 1 FROM organization WHERE id = rid FOR UPDATE;
      v := last_activity_of_organization(rid);
      PERFORM set_config('margince.last_activity_move', 'on', true);
      UPDATE organization SET last_activity_at = v WHERE id = rid;
  END CASE;
  PERFORM set_config('margince.last_activity_move', 'off', true);
END;
$$;;

CREATE FUNCTION organization_open_pipeline_rollup(as_of date)
RETURNS TABLE (
  organization_id uuid,
  open_pipeline_minor_base bigint,
  open_deal_count bigint,
  priced_deal_count bigint
)
LANGUAGE sql
STABLE
SECURITY INVOKER
AS $fn$
 SELECT d.organization_id,
    -- The SUM is bounded too, and separately from its summands. sum(bigint)
    -- answers in numeric, so a set of individually representable deals can add
    -- to a figure no bigint holds — and the reader scans this column into an
    -- int64. Guarding one deal and not the total would have moved the same
    -- failure one level up: the record unreadable because the deals are large
    -- rather than because one of them is.
    --
    -- Out of range answers NULL, which the caller already knows how to report:
    -- the deals were priced, the total cannot be stated, and a figure nobody
    -- can represent is not a figure to publish.
    CASE
      WHEN sum(conv.minor_base)
           BETWEEN -9223372036854775808 AND 9223372036854775807
        THEN sum(conv.minor_base)
      ELSE NULL
    END AS open_pipeline_minor_base,
    count(*) AS open_deal_count,
    -- How many deals actually reached the sum. SUM ignores a null summand
    -- silently, so without this a total covering one of two deals is
    -- indistinguishable from one covering both — a confident figure that is
    -- quietly short, which is worse than the "not computable" it replaces.
    --
    -- It counts the CONVERSION rather than restating when one is possible: a
    -- deal with no rate and a deal whose converted amount does not fit are both
    -- deals the sum did not reach, and one predicate answers for both.
    count(*) FILTER (WHERE conv.minor_base IS NOT NULL) AS priced_deal_count
   FROM deal d
   -- LEFT, not CROSS: an installation whose base-currency row is somehow absent
   -- must still report its open_deal_count. A cross join would return no row at
   -- all for the organization, which reads as "no open deals" — the one answer
   -- that is definitely wrong. With no base currency nothing converts, every
   -- deal contributes null, and the sum is null: not computable, honestly.
   LEFT JOIN LATERAL (
     SELECT (value #>> '{}')::text AS code
       FROM setting
      WHERE key = 'installation.base_currency'
   ) base ON true
   LEFT JOIN LATERAL (
     SELECT r.rate
       FROM fx_rate r
      WHERE d.currency IS DISTINCT FROM base.code
        AND r.from_currency = d.currency
        AND r.to_currency = base.code
        AND r.rate_date <= as_of
      ORDER BY r.rate_date DESC
      LIMIT 1
   ) live ON true
   -- Both currencies' minor-unit scales, each absent for an ordinary two-digit
   -- code and coalesced to ISO's default below. LEFT so a code the table does
   -- not name still converts, at two digits, rather than dropping the deal out
   -- of the sum — an unnamed exception renders wrong for that code, where a
   -- dropped deal silently shortens a total for every code.
   LEFT JOIN currency_minor_digits deal_digits ON deal_digits.currency = d.currency
   LEFT JOIN currency_minor_digits base_digits ON base_digits.currency = base.code
   -- What this deal contributes to the total, or NULL when it contributes
   -- nothing. Three cases, and the bound is written once here so the count
   -- predicate below reads the same answer the sum does.
   LEFT JOIN LATERAL (
     SELECT CASE
       -- Already in the installation's own currency: no rate needed, no scale
       -- to cross, and none should be looked for. This is the ordinary deal.
       WHEN d.currency = base.code THEN d.amount_minor
       -- Converted, and only when the result is a number the column can hold.
       -- The comparison runs in numeric, where the product already is, so it
       -- decides the question BEFORE a cast can raise it.
       --
       -- amount × rate × 10^digits(base) ÷ 10^digits(deal), as ONE expression
       -- so the single round() is the only rounding. numeric is exact, so the
       -- intermediate carries no error to accumulate.
       WHEN live.rate IS NOT NULL
        AND round(d.amount_minor * live.rate
                    * power(10::numeric, coalesce(base_digits.digits, 2))
                    / power(10::numeric, coalesce(deal_digits.digits, 2)))
            BETWEEN -9223372036854775808 AND 9223372036854775807
         THEN round(d.amount_minor * live.rate
                      * power(10::numeric, coalesce(base_digits.digits, 2))
                      / power(10::numeric, coalesce(deal_digits.digits, 2)))::bigint
       -- No usable rate, or a result too large to represent. Nothing is ever
       -- converted at an invented rate of 1 — that would report ¥5,000,000 as
       -- €5,000,000 — and nothing is ever clamped to the biggest number that
       -- fits, which is the same lie with more digits.
       ELSE NULL
     END AS minor_base
   ) conv ON true
  WHERE ((d.status = 'open'::text) AND (d.organization_id IS NOT NULL) AND (d.archived_at IS NULL))
  GROUP BY d.organization_id;
$fn$;;
GRANT EXECUTE ON FUNCTION organization_open_pipeline_rollup(date) TO margince_app;
COMMENT ON FUNCTION organization_open_pipeline_rollup(date) IS
  'Open pipeline per organization in the installation base currency, as of the date the CALLER names. Open deals hold no frozen rate — that happens on close — so each foreign-currency deal converts at the latest fx_rate on or before as_of, across both currencies'' minor-unit scales (currency_minor_digits). The date is a parameter rather than CURRENT_DATE because two readers of one response must not select two different rates: the caller binds it from the same clock every other derived figure reads. A deal with no usable rate, or whose converted amount does not fit a bigint, contributes nothing and is still counted in open_deal_count, so a partial sum is detectable rather than silently short. The total itself answers NULL when it does not fit either.';
