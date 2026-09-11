-- Undo of the contact rename: every statement in the up file, inverted.
--
-- The ORDER is the inverse of the up file's, for the up file's reason. The
-- open vocabularies and the stored values go back while the names are still
-- the new ones, then the names go back, and the function bodies go last --
-- a body naming a column is parsed when the function is created, so
-- recreating one before its column is renamed back fails on the spot.
SET LOCAL lock_timeout = '3s';

-- the vocabularies stored without a CHECK, back
UPDATE communication_suppression
   SET source = 'carried_from_person_consent'
 WHERE source = 'carried_from_contact_consent';

UPDATE webhook_subscription
   SET event_types = (
         SELECT array_agg(replace(t, 'contact.', 'person.') ORDER BY t)
           FROM unnest(event_types) AS t
       )
 WHERE EXISTS (SELECT 1 FROM unnest(event_types) AS t WHERE t LIKE 'contact.%');

UPDATE event_outbox
   SET envelope = jsonb_set(envelope, '{entity,type}', '"person"')
 WHERE published_at IS NULL AND envelope #>> '{entity,type}' = 'contact';
UPDATE event_outbox
   SET stream = 'person.' || substring(stream from 9),
       envelope = jsonb_set(envelope, '{type}',
         to_jsonb('person.' || substring(envelope ->> 'type' from 9)))
 WHERE published_at IS NULL AND stream LIKE 'contact.%';

UPDATE webhook_delivery
   SET event_type = 'person.' || substring(event_type from 9)
 WHERE event_type LIKE 'contact.%';
UPDATE webhook_delivery SET entity_type = 'person' WHERE entity_type = 'contact';

UPDATE approval SET co_target_entity_type = 'person' WHERE co_target_entity_type = 'contact';
UPDATE approval SET target_entity_type = 'person' WHERE target_entity_type = 'contact';

UPDATE field_mask SET object = 'person' WHERE object = 'contact';

UPDATE role
   SET permissions = jsonb_set(permissions, '{objects}',
         ((permissions -> 'objects') - 'contact')
         || jsonb_build_object('person', permissions -> 'objects' -> 'contact'))
 WHERE permissions -> 'objects' ? 'contact';

-- the CHECKs and the rows that store the word, back. The column names below
-- are the ones that exist at this point in the file; the rename that follows
-- carries each constraint expression with it.
ALTER TABLE weekly_plan_commitment DROP CONSTRAINT weekly_plan_commitment_link_type_check;
UPDATE weekly_plan_commitment SET linked_record_type = 'person' WHERE linked_record_type = 'contact';
ALTER TABLE weekly_plan_commitment ADD CONSTRAINT weekly_plan_commitment_link_type_check
  CHECK ((linked_record_type IS NULL) OR linked_record_type IN ('deal', 'lead', 'person', 'company', 'project'));

ALTER TABLE user_record_view DROP CONSTRAINT user_record_view_entity_type_check;
UPDATE user_record_view SET entity_type = 'person' WHERE entity_type = 'contact';
ALTER TABLE user_record_view ADD CONSTRAINT user_record_view_entity_type_check
  CHECK (entity_type IN ('company', 'person'));

ALTER TABLE taggable DROP CONSTRAINT taggable_entity_type_check;
UPDATE taggable SET entity_type = 'person' WHERE entity_type = 'contact';
ALTER TABLE taggable ADD CONSTRAINT taggable_entity_type_check
  CHECK (entity_type IN ('person', 'company', 'deal', 'lead', 'project'));

ALTER TABLE signal DROP CONSTRAINT signal_entity_type_check;
UPDATE signal SET entity_type = 'person' WHERE entity_type = 'contact';
ALTER TABLE signal ADD CONSTRAINT signal_entity_type_check
  CHECK ((entity_type IS NULL) OR entity_type IN ('deal', 'company', 'person', 'project'));

ALTER TABLE saved_view DROP CONSTRAINT saved_view_resource_check;
UPDATE saved_view SET resource = 'people' WHERE resource = 'contacts';
ALTER TABLE saved_view ADD CONSTRAINT saved_view_resource_check
  CHECK (resource IN ('people', 'companies', 'deals', 'activities', 'leads', 'partners', 'projects'));

ALTER TABLE record_grant DROP CONSTRAINT record_grant_record_type_check;
UPDATE record_grant SET record_type = 'person' WHERE record_type = 'contact';
ALTER TABLE record_grant ADD CONSTRAINT record_grant_record_type_check
  CHECK (record_type IN ('person', 'company', 'deal', 'lead', 'project'));

DROP INDEX provider_run_one_live_contact_fingerprint;
DROP INDEX provider_run_contact_history;
ALTER TABLE provider_run DROP CONSTRAINT provider_run_subject_kind_check;
ALTER TABLE provider_run DROP CONSTRAINT provider_run_subject_shape;
UPDATE provider_run SET subject_kind = 'person' WHERE subject_kind = 'contact';
ALTER TABLE provider_run ADD CONSTRAINT provider_run_subject_kind_check
  CHECK (subject_kind IN ('person', 'scrubbed'));
ALTER TABLE provider_run ADD CONSTRAINT provider_run_subject_shape
  CHECK (((subject_kind = 'person') AND (contact_id IS NOT NULL)) OR ((subject_kind = 'scrubbed') AND (contact_id IS NULL)));
CREATE UNIQUE INDEX provider_run_one_live_person_fingerprint ON provider_run (contact_id, provider, input_fingerprint)
  WHERE ((subject_kind = 'person') AND (state IN ('queued', 'submitting', 'in_progress', 'submission_unknown')));
CREATE INDEX provider_run_person_history ON provider_run (contact_id, provider, created_at DESC)
  WHERE (subject_kind = 'person');

ALTER TABLE provider_applied_field DROP CONSTRAINT provider_applied_field_target_check;
UPDATE provider_applied_field SET target_table = 'person' WHERE target_table = 'contact';
UPDATE provider_applied_field SET target_table = 'person_social' WHERE target_table = 'contact_social';
UPDATE provider_applied_field SET target_table = 'person_email' WHERE target_table = 'contact_email';
UPDATE provider_applied_field SET target_table = 'person_phone' WHERE target_table = 'contact_phone';
ALTER TABLE provider_applied_field ADD CONSTRAINT provider_applied_field_target_check
  CHECK (target_table IN ('person', 'person_social', 'person_email', 'person_phone', 'relationship'));

ALTER TABLE list_member DROP CONSTRAINT list_member_entity_type_check;
UPDATE list_member SET entity_type = 'person' WHERE entity_type = 'contact';
ALTER TABLE list_member ADD CONSTRAINT list_member_entity_type_check
  CHECK (entity_type IN ('person', 'company', 'deal', 'lead', 'project'));

ALTER TABLE list DROP CONSTRAINT list_entity_type_check;
UPDATE list SET entity_type = 'person' WHERE entity_type = 'contact';
ALTER TABLE list ADD CONSTRAINT list_entity_type_check
  CHECK (entity_type IN ('person', 'company', 'deal', 'lead', 'project'));

ALTER TABLE field_provenance DROP CONSTRAINT field_provenance_object_type_check;
UPDATE field_provenance SET object_type = 'person' WHERE object_type = 'contact';
ALTER TABLE field_provenance ADD CONSTRAINT field_provenance_object_type_check
  CHECK (object_type IN ('person', 'company', 'deal', 'lead', 'activity', 'project', 'relationship', 'partner'));

ALTER TABLE embedding DROP CONSTRAINT embedding_entity_type_check;
UPDATE embedding SET entity_type = 'person' WHERE entity_type = 'contact';
ALTER TABLE embedding ADD CONSTRAINT embedding_entity_type_check
  CHECK (entity_type IN ('person', 'company', 'deal', 'lead', 'activity', 'project', 'relationship', 'partner'));

ALTER TABLE dedupe_candidate DROP CONSTRAINT dedupe_candidate_entity_type_check;
ALTER TABLE dedupe_candidate DROP CONSTRAINT dedupe_candidate_shape;
UPDATE dedupe_candidate SET entity_type = 'person' WHERE entity_type = 'contact';
ALTER TABLE dedupe_candidate ADD CONSTRAINT dedupe_candidate_entity_type_check
  CHECK (entity_type IN ('person', 'company', 'lead'));
ALTER TABLE dedupe_candidate ADD CONSTRAINT dedupe_candidate_shape
  CHECK (((entity_type = 'person') AND (left_contact_id IS NOT NULL) AND (right_contact_id IS NOT NULL) AND (left_company_id IS NULL) AND (right_company_id IS NULL) AND (left_lead_id IS NULL) AND (right_lead_id IS NULL)) OR ((entity_type = 'company') AND (left_company_id IS NOT NULL) AND (right_company_id IS NOT NULL) AND (left_contact_id IS NULL) AND (right_contact_id IS NULL) AND (left_lead_id IS NULL) AND (right_lead_id IS NULL)) OR ((entity_type = 'lead') AND (left_lead_id IS NOT NULL) AND (right_lead_id IS NOT NULL) AND (left_contact_id IS NULL) AND (right_contact_id IS NULL) AND (left_company_id IS NULL) AND (right_company_id IS NULL)));

ALTER TABLE custom_field DROP CONSTRAINT custom_field_object_check;
UPDATE custom_field SET object = 'person' WHERE object = 'contact';
ALTER TABLE custom_field ADD CONSTRAINT custom_field_object_check
  CHECK (object IN ('person', 'company', 'deal', 'lead', 'activity', 'project', 'relationship', 'partner'));

ALTER TABLE communication_decision DROP CONSTRAINT communication_decision_subject_kind;
UPDATE communication_decision SET subject_kind = 'person' WHERE subject_kind = 'contact';
ALTER TABLE communication_decision ADD CONSTRAINT communication_decision_subject_kind
  CHECK ((subject_kind IS NULL) OR subject_kind IN ('person', 'lead'));

ALTER TABLE capture_pending_counterparty DROP CONSTRAINT capture_pending_counterparty_kind_check;
UPDATE capture_pending_counterparty SET kind = 'person' WHERE kind = 'contact';
ALTER TABLE capture_pending_counterparty ADD CONSTRAINT capture_pending_counterparty_kind_check
  CHECK ((kind IS NULL) OR kind IN ('person', 'role_mailbox', 'company_sender', 'newsletter', 'transactional', 'spam', 'personal', 'advisor'));

ALTER TABLE capture_backfill_creation DROP CONSTRAINT capture_backfill_creation_kind;
UPDATE capture_backfill_creation SET kind = 'person' WHERE kind = 'contact';
ALTER TABLE capture_backfill_creation ADD CONSTRAINT capture_backfill_creation_kind
  CHECK (kind IN ('person', 'company_queued'));

ALTER TABLE attachment DROP CONSTRAINT attachment_entity_type_check;
UPDATE attachment SET entity_type = 'person' WHERE entity_type = 'contact';
ALTER TABLE attachment ADD CONSTRAINT attachment_entity_type_check
  CHECK (entity_type IN ('person', 'company', 'deal', 'lead', 'activity', 'project', 'relationship', 'partner'));

ALTER TABLE ai_task_run DROP CONSTRAINT ai_task_run_quantity_unit_check;
UPDATE ai_task_run SET quantity_unit = 'people' WHERE quantity_unit = 'contacts';
ALTER TABLE ai_task_run ADD CONSTRAINT ai_task_run_quantity_unit_check
  CHECK ((quantity_unit IS NULL) OR quantity_unit IN ('messages', 'records', 'people', 'documents'));

ALTER TABLE ai_feedback DROP CONSTRAINT ai_feedback_subject_type_check;
UPDATE ai_feedback SET subject_type = 'person' WHERE subject_type = 'contact';
ALTER TABLE ai_feedback ADD CONSTRAINT ai_feedback_subject_type_check
  CHECK (subject_type IN ('company', 'person', 'deal', 'lead'));

ALTER TABLE activity_link DROP CONSTRAINT activity_link_entity_type_check;
ALTER TABLE activity_link DROP CONSTRAINT activity_link_shape;
UPDATE activity_link SET entity_type = 'person' WHERE entity_type = 'contact';
ALTER TABLE activity_link ADD CONSTRAINT activity_link_entity_type_check
  CHECK (entity_type IN ('person', 'company', 'deal', 'lead', 'project'));
ALTER TABLE activity_link ADD CONSTRAINT activity_link_shape
  CHECK (((entity_type = 'person') AND (contact_id IS NOT NULL) AND (company_id IS NULL) AND (deal_id IS NULL) AND (lead_id IS NULL) AND (project_id IS NULL)) OR ((entity_type = 'company') AND (company_id IS NOT NULL) AND (contact_id IS NULL) AND (deal_id IS NULL) AND (lead_id IS NULL) AND (project_id IS NULL)) OR ((entity_type = 'deal') AND (deal_id IS NOT NULL) AND (contact_id IS NULL) AND (company_id IS NULL) AND (lead_id IS NULL) AND (project_id IS NULL)) OR ((entity_type = 'lead') AND (lead_id IS NOT NULL) AND (contact_id IS NULL) AND (company_id IS NULL) AND (deal_id IS NULL) AND (project_id IS NULL)) OR ((entity_type = 'project') AND (project_id IS NOT NULL) AND (contact_id IS NULL) AND (company_id IS NULL) AND (deal_id IS NULL) AND (lead_id IS NULL)));

-- triggers, back
ALTER TRIGGER trg_contact_updated ON contact RENAME TO trg_person_updated;
ALTER TRIGGER trg_contact_channel_identity_updated ON contact_channel_identity RENAME TO trg_person_channel_identity_updated;
ALTER TRIGGER trg_contact_email_updated ON contact_email RENAME TO trg_person_email_updated;
ALTER TRIGGER trg_contact_phone_updated ON contact_phone RENAME TO trg_person_phone_updated;
ALTER TRIGGER trg_contact_profile_field_updated ON contact_profile_field RENAME TO trg_person_profile_field_updated;

-- indexes, back
ALTER INDEX communication_basis_live_contact RENAME TO communication_basis_live_person;
ALTER INDEX communication_suppression_live_contact RENAME TO communication_suppression_live_person;
ALTER INDEX consent_qualifying_event_contact_ix RENAME TO consent_qualifying_event_person_ix;
ALTER INDEX conversation_claim_contact_ix RENAME TO conversation_claim_person_ix;
ALTER INDEX idx_alink_contact RENAME TO idx_alink_person;
ALTER INDEX idx_aparticipant_contact RENAME TO idx_aparticipant_person;
ALTER INDEX idx_consent_doi_token_contact RENAME TO idx_consent_doi_token_person;
ALTER INDEX idx_consent_event_contact RENAME TO idx_consent_event_person;
ALTER INDEX idx_graph_edge_contact RENAME TO idx_graph_edge_person;
ALTER INDEX idx_contact_channel_identity_contact RENAME TO idx_person_channel_identity_person;
ALTER INDEX idx_contact_created_keyset RENAME TO idx_person_created_keyset;
ALTER INDEX idx_contact_email_correspondence RENAME TO idx_person_email_correspondence;
ALTER INDEX idx_contact_email_contact RENAME TO idx_person_email_person;
ALTER INDEX idx_contact_from_lead RENAME TO idx_person_from_lead;
ALTER INDEX idx_contact_last_activity_keyset RENAME TO idx_person_last_activity_keyset;
ALTER INDEX idx_contact_merged_into RENAME TO idx_person_merged_into;
ALTER INDEX idx_contact_name_keyset RENAME TO idx_person_name_keyset;
ALTER INDEX idx_contact_name_keyset_desc RENAME TO idx_person_name_keyset_desc;
ALTER INDEX idx_contact_name_trgm RENAME TO idx_person_name_trgm;
ALTER INDEX idx_contact_owner RENAME TO idx_person_owner;
ALTER INDEX idx_contact_phone_contact RENAME TO idx_person_phone_person;
ALTER INDEX idx_contact_profile_field RENAME TO idx_person_profile_field;
ALTER INDEX idx_contact_search RENAME TO idx_person_search;
ALTER INDEX idx_contact_social_contact RENAME TO idx_person_social_person;
ALTER INDEX idx_contact_updated_keyset RENAME TO idx_person_updated_keyset;
ALTER INDEX idx_rel_employer_contacts RENAME TO idx_rel_employer_people;
ALTER INDEX idx_rel_history_contact RENAME TO idx_rel_history_person;
ALTER INDEX idx_rel_company_contacts RENAME TO idx_rel_company_people;
ALTER INDEX idx_rel_contact_companies RENAME TO idx_rel_person_companies;
ALTER INDEX idx_rel_contact_projects RENAME TO idx_rel_person_projects;
ALTER INDEX idx_rel_traverse_contact RENAME TO idx_rel_traverse_person;
ALTER INDEX idx_rel_works_with_contact RENAME TO idx_rel_works_with_person;
ALTER INDEX idx_sdr_handoff_contact RENAME TO idx_sdr_handoff_person;
ALTER INDEX idx_withdrawal_credential_contact RENAME TO idx_withdrawal_credential_person;
ALTER INDEX intro_request_for_contact RENAME TO intro_request_for_person;
ALTER INDEX ix_confirm_token_contact RENAME TO ix_confirm_token_person;
ALTER INDEX ix_contact_confirm_submission_open RENAME TO ix_person_confirm_submission_open;
ALTER INDEX ix_contact_confirm_submission_contact RENAME TO ix_person_confirm_submission_person;
ALTER INDEX contact_acquisition_evidence_by_contact RENAME TO person_acquisition_evidence_by_person;
ALTER INDEX contact_brief_contact_ix RENAME TO person_brief_person_ix;
ALTER INDEX contact_moment_dismissal_contact_ix RENAME TO person_moment_dismissal_person_ix;
ALTER INDEX contact_provider_claim_latest RENAME TO person_provider_claim_latest;
ALTER INDEX provider_applied_field_by_contact RENAME TO provider_applied_field_by_person;
ALTER INDEX uq_contact_channel_identity RENAME TO uq_person_channel_identity;
ALTER INDEX uq_contact_email_dedupe RENAME TO uq_person_email_dedupe;
ALTER INDEX uq_contact_email_primary RENAME TO uq_person_email_primary;
ALTER INDEX uq_contact_phone_primary RENAME TO uq_person_phone_primary;
ALTER INDEX uq_preference_token_contact_address RENAME TO uq_preference_token_person_address;
ALTER INDEX uq_rel_deal_contact_role RENAME TO uq_rel_deal_person_role;

-- constraints, back
ALTER TABLE activity_link RENAME CONSTRAINT activity_link_contact_id_fkey TO activity_link_person_id_fkey;
ALTER TABLE activity_participant RENAME CONSTRAINT activity_participant_contact_fkey TO activity_participant_person_fkey;
ALTER TABLE communication_basis RENAME CONSTRAINT communication_basis_contact_id_fkey TO communication_basis_person_id_fkey;
ALTER TABLE communication_suppression RENAME CONSTRAINT communication_suppression_contact_id_fkey TO communication_suppression_person_id_fkey;
ALTER TABLE confirm_token RENAME CONSTRAINT confirm_token_contact_fkey TO confirm_token_person_fkey;
ALTER TABLE consent_doi_token RENAME CONSTRAINT consent_doi_token_contact_id_fkey TO consent_doi_token_person_id_fkey;
ALTER TABLE consent_event RENAME CONSTRAINT consent_event_contact_id_fkey TO consent_event_person_id_fkey;
ALTER TABLE consent_existing_customer_flag RENAME CONSTRAINT consent_existing_customer_contact_fkey TO consent_existing_customer_person_fkey;
ALTER TABLE consent_qualifying_event RENAME CONSTRAINT consent_qualifying_event_contact_fkey TO consent_qualifying_event_person_fkey;
ALTER TABLE conversation_claim RENAME CONSTRAINT conversation_claim_contact_fkey TO conversation_claim_person_fkey;
ALTER TABLE data_subject_request RENAME CONSTRAINT data_subject_request_contact_id_fkey TO data_subject_request_person_id_fkey;
ALTER TABLE dedupe_candidate RENAME CONSTRAINT dedupe_candidate_left_contact_id_fkey TO dedupe_candidate_left_person_id_fkey;
ALTER TABLE dedupe_candidate RENAME CONSTRAINT dedupe_candidate_right_contact_id_fkey TO dedupe_candidate_right_person_id_fkey;
ALTER TABLE graph_interaction_edge RENAME CONSTRAINT graph_interaction_edge_workspace_id_contact_id_fkey TO graph_interaction_edge_workspace_id_person_id_fkey;
ALTER TABLE intro_request RENAME CONSTRAINT intro_request_contact_id_fkey TO intro_request_person_id_fkey;
ALTER TABLE intro_request RENAME CONSTRAINT intro_request_through_contact_id_fkey TO intro_request_through_person_id_fkey;
ALTER TABLE lead RENAME CONSTRAINT lead_promoted_contact_id_fkey TO lead_promoted_person_id_fkey;
ALTER TABLE linkedin_connection RENAME CONSTRAINT linkedin_connection_workspace_id_matched_contact_id_fkey TO linkedin_connection_workspace_id_matched_person_id_fkey;
ALTER TABLE contact RENAME CONSTRAINT contact_converted_from_lead_id_fkey TO person_converted_from_lead_id_fkey;
ALTER TABLE contact RENAME CONSTRAINT contact_merged_into_id_fkey TO person_merged_into_id_fkey;
ALTER TABLE contact RENAME CONSTRAINT contact_owner_id_fkey TO person_owner_id_fkey;
ALTER TABLE contact RENAME CONSTRAINT contact_owner_private_names_its_owner TO person_owner_private_names_its_owner;
ALTER TABLE contact RENAME CONSTRAINT contact_pkey TO person_pkey;
ALTER TABLE contact RENAME CONSTRAINT contact_visibility_check TO person_visibility_check;
ALTER TABLE contact_acquisition_evidence RENAME CONSTRAINT contact_acquisition_evidence_kind TO person_acquisition_evidence_kind;
ALTER TABLE contact_acquisition_evidence RENAME CONSTRAINT contact_acquisition_evidence_contact_id_fkey TO person_acquisition_evidence_person_id_fkey;
ALTER TABLE contact_acquisition_evidence RENAME CONSTRAINT contact_acquisition_evidence_pkey TO person_acquisition_evidence_pkey;
ALTER TABLE contact_acquisition_evidence RENAME CONSTRAINT contact_acquisition_evidence_source_shape TO person_acquisition_evidence_source_shape;
ALTER TABLE contact_brief RENAME CONSTRAINT contact_brief_generated_by_check TO person_brief_generated_by_check;
ALTER TABLE contact_brief RENAME CONSTRAINT contact_brief_contact_fkey TO person_brief_person_fkey;
ALTER TABLE contact_brief RENAME CONSTRAINT contact_brief_pkey TO person_brief_pkey;
ALTER TABLE contact_brief RENAME CONSTRAINT contact_brief_user_fkey TO person_brief_user_fkey;
ALTER TABLE contact_channel_identity RENAME CONSTRAINT contact_channel_identity_contact_id_fkey TO person_channel_identity_person_id_fkey;
ALTER TABLE contact_channel_identity RENAME CONSTRAINT contact_channel_identity_pkey TO person_channel_identity_pkey;
ALTER TABLE contact_channel_identity RENAME CONSTRAINT contact_channel_identity_provider_fkey TO person_channel_identity_provider_fkey;
ALTER TABLE contact_confirm_submission RENAME CONSTRAINT contact_confirm_submission_field_matches_kind TO person_confirm_submission_field_matches_kind;
ALTER TABLE contact_confirm_submission RENAME CONSTRAINT contact_confirm_submission_kind_check TO person_confirm_submission_kind_check;
ALTER TABLE contact_confirm_submission RENAME CONSTRAINT contact_confirm_submission_contact_fkey TO person_confirm_submission_person_fkey;
ALTER TABLE contact_confirm_submission RENAME CONSTRAINT contact_confirm_submission_pkey TO person_confirm_submission_pkey;
ALTER TABLE contact_confirm_submission RENAME CONSTRAINT contact_confirm_submission_resolution_check TO person_confirm_submission_resolution_check;
ALTER TABLE contact_confirm_submission RENAME CONSTRAINT contact_confirm_submission_resolved_together TO person_confirm_submission_resolved_together;
ALTER TABLE contact_confirm_submission RENAME CONSTRAINT contact_confirm_submission_token_fkey TO person_confirm_submission_token_fkey;
ALTER TABLE contact_consent RENAME CONSTRAINT contact_consent_lead_id_fkey TO person_consent_lead_id_fkey;
ALTER TABLE contact_consent RENAME CONSTRAINT contact_consent_lead_unique TO person_consent_lead_unique;
ALTER TABLE contact_consent RENAME CONSTRAINT contact_consent_contact_id_fkey TO person_consent_person_id_fkey;
ALTER TABLE contact_consent RENAME CONSTRAINT contact_consent_pkey TO person_consent_pkey;
ALTER TABLE contact_consent RENAME CONSTRAINT contact_consent_purpose_id_fkey TO person_consent_purpose_id_fkey;
ALTER TABLE contact_consent RENAME CONSTRAINT contact_consent_state_check TO person_consent_state_check;
ALTER TABLE contact_consent RENAME CONSTRAINT contact_consent_subject TO person_consent_subject;
ALTER TABLE contact_consent RENAME CONSTRAINT contact_consent_unique TO person_consent_unique;
ALTER TABLE contact_email RENAME CONSTRAINT contact_email_email_type_check TO person_email_email_type_check;
ALTER TABLE contact_email RENAME CONSTRAINT contact_email_norm TO person_email_norm;
ALTER TABLE contact_email RENAME CONSTRAINT contact_email_contact_id_fkey TO person_email_person_id_fkey;
ALTER TABLE contact_email RENAME CONSTRAINT contact_email_pkey TO person_email_pkey;
ALTER TABLE contact_moment_dismissal RENAME CONSTRAINT contact_moment_dismissal_contact_fkey TO person_moment_dismissal_person_fkey;
ALTER TABLE contact_moment_dismissal RENAME CONSTRAINT contact_moment_dismissal_pkey TO person_moment_dismissal_pkey;
ALTER TABLE contact_moment_dismissal RENAME CONSTRAINT contact_moment_dismissal_user_fkey TO person_moment_dismissal_user_fkey;
ALTER TABLE contact_phone RENAME CONSTRAINT contact_phone_contact_id_fkey TO person_phone_person_id_fkey;
ALTER TABLE contact_phone RENAME CONSTRAINT contact_phone_phone_type_check TO person_phone_phone_type_check;
ALTER TABLE contact_phone RENAME CONSTRAINT contact_phone_pkey TO person_phone_pkey;
ALTER TABLE contact_phone RENAME CONSTRAINT contact_phone_superseded_phone_id_fkey TO person_phone_superseded_phone_id_fkey;
ALTER TABLE contact_profile_field RENAME CONSTRAINT contact_profile_field_confidence_check TO person_profile_field_confidence_check;
ALTER TABLE contact_profile_field RENAME CONSTRAINT contact_profile_field_field_check TO person_profile_field_field_check;
ALTER TABLE contact_profile_field RENAME CONSTRAINT contact_profile_field_contact_fk TO person_profile_field_person_fk;
ALTER TABLE contact_profile_field RENAME CONSTRAINT contact_profile_field_pkey TO person_profile_field_pkey;
ALTER TABLE contact_profile_field RENAME CONSTRAINT uq_contact_profile_field TO uq_person_profile_field;
ALTER TABLE contact_provider_claim RENAME CONSTRAINT contact_provider_claim_claim_key_check TO person_provider_claim_claim_key_check;
ALTER TABLE contact_provider_claim RENAME CONSTRAINT contact_provider_claim_confidence_check TO person_provider_claim_confidence_check;
ALTER TABLE contact_provider_claim RENAME CONSTRAINT contact_provider_claim_contact_id_fkey TO person_provider_claim_person_id_fkey;
ALTER TABLE contact_provider_claim RENAME CONSTRAINT contact_provider_claim_pkey TO person_provider_claim_pkey;
ALTER TABLE contact_provider_claim RENAME CONSTRAINT contact_provider_claim_run_id_claim_key_key TO person_provider_claim_run_id_claim_key_key;
ALTER TABLE contact_provider_claim RENAME CONSTRAINT contact_provider_claim_run_id_fkey TO person_provider_claim_run_id_fkey;
ALTER TABLE contact_signature_enrich_state RENAME CONSTRAINT contact_signature_enrich_state_activity_id_fkey TO person_signature_enrich_state_activity_id_fkey;
ALTER TABLE contact_signature_enrich_state RENAME CONSTRAINT contact_signature_enrich_state_contact_id_fkey TO person_signature_enrich_state_person_id_fkey;
ALTER TABLE contact_signature_enrich_state RENAME CONSTRAINT contact_signature_enrich_state_pkey TO person_signature_enrich_state_pkey;
ALTER TABLE contact_social RENAME CONSTRAINT contact_social_contact_id_fkey TO person_social_person_id_fkey;
ALTER TABLE contact_social RENAME CONSTRAINT contact_social_contact_id_platform_key TO person_social_person_id_platform_key;
ALTER TABLE contact_social RENAME CONSTRAINT contact_social_pkey TO person_social_pkey;
ALTER TABLE preference_token RENAME CONSTRAINT preference_token_contact_email_id_fkey TO preference_token_person_email_id_fkey;
ALTER TABLE preference_token RENAME CONSTRAINT preference_token_contact_fkey TO preference_token_person_fkey;
ALTER TABLE privacy_notice_case RENAME CONSTRAINT privacy_notice_case_contact_fkey TO privacy_notice_case_person_fkey;
ALTER TABLE provider_applied_field RENAME CONSTRAINT provider_applied_field_contact_id_fkey TO provider_applied_field_person_id_fkey;
ALTER TABLE provider_run RENAME CONSTRAINT provider_run_contact_id_fkey TO provider_run_person_id_fkey;
ALTER TABLE relationship RENAME CONSTRAINT relationship_counterparty_contact_id_fkey TO relationship_counterparty_person_id_fkey;
ALTER TABLE relationship RENAME CONSTRAINT relationship_contact_id_fkey TO relationship_person_id_fkey;
ALTER TABLE relationship_nudge_dismissal RENAME CONSTRAINT relationship_nudge_dismissal_contact_id_fkey TO relationship_nudge_dismissal_person_id_fkey;
ALTER TABLE sdr_handoff RENAME CONSTRAINT sdr_handoff_contact_id_fkey TO sdr_handoff_person_id_fkey;
ALTER TABLE signal RENAME CONSTRAINT signal_resolved_contact_fkey TO signal_resolved_person_fkey;
ALTER TABLE withdrawal_credential RENAME CONSTRAINT withdrawal_credential_contact_id_fkey TO withdrawal_credential_person_id_fkey;

-- columns, back
ALTER TABLE activity_link RENAME COLUMN contact_id TO person_id;
ALTER TABLE activity_participant RENAME COLUMN contact_id TO person_id;
ALTER TABLE capture_backfill RENAME COLUMN contacts_created TO people_created;
ALTER TABLE communication_basis RENAME COLUMN contact_id TO person_id;
ALTER TABLE communication_suppression RENAME COLUMN contact_id TO person_id;
ALTER TABLE confirm_token RENAME COLUMN contact_id TO person_id;
ALTER TABLE consent_doi_token RENAME COLUMN contact_id TO person_id;
ALTER TABLE consent_event RENAME COLUMN contact_id TO person_id;
ALTER TABLE consent_existing_customer_flag RENAME COLUMN contact_id TO person_id;
ALTER TABLE consent_qualifying_event RENAME COLUMN contact_id TO person_id;
ALTER TABLE conversation_claim RENAME COLUMN contact_id TO person_id;
ALTER TABLE data_subject_request RENAME COLUMN contact_id TO person_id;
ALTER TABLE dedupe_candidate RENAME COLUMN left_contact_id TO left_person_id;
ALTER TABLE dedupe_candidate RENAME COLUMN right_contact_id TO right_person_id;
ALTER TABLE graph_contact_edge RENAME COLUMN contact_a TO person_a;
ALTER TABLE graph_contact_edge RENAME COLUMN contact_b TO person_b;
ALTER TABLE graph_interaction_edge RENAME COLUMN contact_id TO person_id;
ALTER TABLE intro_request RENAME COLUMN contact_id TO person_id;
ALTER TABLE intro_request RENAME COLUMN through_contact_id TO through_person_id;
ALTER TABLE lead RENAME COLUMN promoted_contact_id TO promoted_person_id;
ALTER TABLE linkedin_connection RENAME COLUMN matched_contact_id TO matched_person_id;
ALTER TABLE contact_acquisition_evidence RENAME COLUMN contact_id TO person_id;
ALTER TABLE contact_brief RENAME COLUMN contact_id TO person_id;
ALTER TABLE contact_channel_identity RENAME COLUMN contact_id TO person_id;
ALTER TABLE contact_confirm_submission RENAME COLUMN contact_id TO person_id;
ALTER TABLE contact_consent RENAME COLUMN contact_id TO person_id;
ALTER TABLE contact_email RENAME COLUMN contact_id TO person_id;
ALTER TABLE contact_moment_dismissal RENAME COLUMN contact_id TO person_id;
ALTER TABLE contact_phone RENAME COLUMN contact_id TO person_id;
ALTER TABLE contact_profile_field RENAME COLUMN contact_id TO person_id;
ALTER TABLE contact_provider_claim RENAME COLUMN contact_id TO person_id;
ALTER TABLE contact_signature_enrich_state RENAME COLUMN contact_id TO person_id;
ALTER TABLE contact_social RENAME COLUMN contact_id TO person_id;
ALTER TABLE preference_token RENAME COLUMN contact_email_id TO person_email_id;
ALTER TABLE preference_token RENAME COLUMN contact_id TO person_id;
ALTER TABLE privacy_notice_case RENAME COLUMN contact_id TO person_id;
ALTER TABLE provider_applied_field RENAME COLUMN contact_id TO person_id;
ALTER TABLE provider_run RENAME COLUMN contact_id TO person_id;
ALTER TABLE relationship RENAME COLUMN counterparty_contact_id TO counterparty_person_id;
ALTER TABLE relationship RENAME COLUMN contact_id TO person_id;
ALTER TABLE relationship_nudge_dismissal RENAME COLUMN contact_id TO person_id;
ALTER TABLE sdr_handoff RENAME COLUMN contact_id TO person_id;
ALTER TABLE signal RENAME COLUMN resolved_contact_id TO resolved_person_id;
ALTER TABLE site_read RENAME COLUMN contacts TO people;
ALTER TABLE withdrawal_credential RENAME COLUMN contact_id TO person_id;

-- tables, back
ALTER TABLE contact RENAME TO person;
ALTER TABLE contact_acquisition_evidence RENAME TO person_acquisition_evidence;
ALTER TABLE contact_brief RENAME TO person_brief;
ALTER TABLE contact_channel_identity RENAME TO person_channel_identity;
ALTER TABLE contact_confirm_submission RENAME TO person_confirm_submission;
ALTER TABLE contact_consent RENAME TO person_consent;
ALTER TABLE contact_email RENAME TO person_email;
ALTER TABLE contact_moment_dismissal RENAME TO person_moment_dismissal;
ALTER TABLE contact_phone RENAME TO person_phone;
ALTER TABLE contact_profile_field RENAME TO person_profile_field;
ALTER TABLE contact_provider_claim RENAME TO person_provider_claim;
ALTER TABLE contact_signature_enrich_state RENAME TO person_signature_enrich_state;
ALTER TABLE contact_social RENAME TO person_social;
-- the function bodies, back. Last, because the names above have just moved
-- and a body is parsed when the function is created: recreating one that
-- names person_id before that column exists again fails on the spot.
DROP FUNCTION last_activity_of_contact(uuid);

CREATE FUNCTION last_activity_of_person(pid uuid) RETURNS timestamptz
    LANGUAGE sql STABLE
    AS $$
  SELECT max(a.occurred_at)
    FROM activity_link l
    JOIN activity a ON a.id = l.activity_id AND a.archived_at IS NULL
     AND a.audience = 'workspace'
     AND a.origin NOT IN ('system_remediation', 'system_notice')
   WHERE l.person_id = pid
$$;

CREATE OR REPLACE FUNCTION last_activity_of_company(cid uuid) RETURNS timestamptz
    LANGUAGE sql STABLE
    AS $$
  SELECT max(v) FROM (
    -- Filed against the account itself.
    SELECT max(a.occurred_at) AS v
      FROM activity_link l
      JOIN activity a ON a.id = l.activity_id AND a.archived_at IS NULL
       AND a.audience = 'workspace'
       AND a.origin NOT IN ('system_remediation', 'system_notice')
     WHERE l.company_id = cid
    UNION ALL
    -- Filed against one of its deals.
    SELECT max(a.occurred_at)
      FROM deal d
      JOIN activity_link l ON l.deal_id = d.id
      JOIN activity a ON a.id = l.activity_id AND a.archived_at IS NULL
       AND a.audience = 'workspace'
       AND a.origin NOT IN ('system_remediation', 'system_notice')
     WHERE d.company_id = cid
    UNION ALL
    -- Filed against a contact it currently employs.
    SELECT max(a.occurred_at)
      FROM relationship r
      JOIN activity_link l ON l.person_id = r.person_id
      JOIN activity a ON a.id = l.activity_id AND a.archived_at IS NULL
       AND a.audience = 'workspace'
       AND a.origin NOT IN ('system_remediation', 'system_notice')
     WHERE r.company_id = cid AND r.kind = 'employment'
       AND r.ended_at IS NULL AND r.archived_at IS NULL
  ) arms
$$;

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
    WHEN 'company'::regclass THEN
      PERFORM 1 FROM company WHERE id = rid FOR UPDATE;
      v := last_activity_of_company(rid);
      PERFORM set_config('margince.last_activity_move', 'on', true);
      UPDATE company SET last_activity_at = v WHERE id = rid;
  END CASE;
  PERFORM set_config('margince.last_activity_move', 'off', true);
END;
$$;

DROP FUNCTION refresh_last_activity_for_link(uuid, uuid, uuid);

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
       UNION SELECT d.company_id FROM deal d WHERE d.id = did AND d.company_id IS NOT NULL
       UNION SELECT r.company_id FROM relationship r
              WHERE r.person_id = cid AND r.kind = 'employment' AND r.ended_at IS NULL AND r.archived_at IS NULL
     ) reach ORDER BY x
  LOOP
    PERFORM move_last_activity('company', reached);
  END LOOP;
END;
$$;

CREATE OR REPLACE FUNCTION trg_activity_last_activity() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
  PERFORM refresh_last_activity_for_link(l.person_id, l.deal_id, l.company_id)
     FROM activity_link l WHERE l.activity_id = NEW.id;
  RETURN NULL;
END;
$$;

CREATE OR REPLACE FUNCTION trg_activity_link_last_activity() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
  IF TG_OP IN ('DELETE', 'UPDATE') THEN
    PERFORM refresh_last_activity_for_link(OLD.person_id, OLD.deal_id, OLD.company_id);
  END IF;
  IF TG_OP IN ('INSERT', 'UPDATE') THEN
    PERFORM refresh_last_activity_for_link(NEW.person_id, NEW.deal_id, NEW.company_id);
  END IF;
  RETURN NULL;
END;
$$;

CREATE OR REPLACE FUNCTION activity_link_refuses_a_company_meeting()
RETURNS trigger
LANGUAGE plpgsql
AS $$
DECLARE
    activity_kind text;
BEGIN
    IF NEW.entity_type <> 'company' THEN
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
               WHERE activity_id = NEW.id AND entity_type = 'company') THEN
        RAISE EXCEPTION
            'this activity is linked to a company, so it cannot become a %: a % is with a person',
            NEW.kind, NEW.kind
            USING ERRCODE = 'check_violation';
    END IF;

    RETURN NEW;
END;
$$;
