-- The record is called a contact.
--
-- The product says contact on every surface a person reads -- the sidebar, the
-- URL, the page heading -- while the schema says `person`, the contract says
-- `/people`, the public events say `person.created` and the search DSL says
-- `persons`. Four words for one record. A reader grepping for the contact table
-- finds nothing; a caller reading `GET /people/{id}` behind a `#/contacts/:id`
-- URL has to learn that they are the same thing. Nothing failed, because
-- nothing was looking.
--
-- Renames, not rewrites: every table, column, constraint, index, trigger and
-- function carrying the old word moves to the new one, and the rows that STORE
-- the word as a value ('person' in an entity_type, 'people' in a saved view's
-- resource) move with them.
--
-- What keeps the old letters, because it is not this record: `personal` (a
-- counterparty verdict, a mail posture, an exclusion), `in_person` (a
-- qualifying event's kind, where the word means physically present),
-- `personality_md` (a voice profile's prose) and `personnel`.
--
-- graph_contact_edge keeps its name and takes new columns. It was already the
-- contact-to-contact sibling of graph_interaction_edge; with person_a and
-- person_b renamed it reads as what it is, and inventing a third word for the
-- table would put one back.
--
-- audit_log is not touched, because it cannot be: trg_audit_no_mutate refuses
-- an UPDATE on it, which is the property that makes the trail worth having. So
-- rows written before this migration keep `entity_type = 'person'` forever, and
-- the readers that filter the trail by record type accept both words --
-- compose/auditlegacytype.go says so once, beside them.
--
-- This takes ACCESS EXCLUSIVE on most of the schema for one transaction. The
-- timeout below bounds the WAIT for each lock and not the HOLD: once granted,
-- every lock is held until the migration commits and every reader queues behind
-- it. That is the cost of a rename, stated rather than mitigated.
SET LOCAL lock_timeout = '3s';


-- 1. tables
ALTER TABLE person RENAME TO contact;
ALTER TABLE person_acquisition_evidence RENAME TO contact_acquisition_evidence;
ALTER TABLE person_brief RENAME TO contact_brief;
ALTER TABLE person_channel_identity RENAME TO contact_channel_identity;
ALTER TABLE person_confirm_submission RENAME TO contact_confirm_submission;
ALTER TABLE person_consent RENAME TO contact_consent;
ALTER TABLE person_email RENAME TO contact_email;
ALTER TABLE person_moment_dismissal RENAME TO contact_moment_dismissal;
ALTER TABLE person_phone RENAME TO contact_phone;
ALTER TABLE person_profile_field RENAME TO contact_profile_field;
ALTER TABLE person_provider_claim RENAME TO contact_provider_claim;
ALTER TABLE person_signature_enrich_state RENAME TO contact_signature_enrich_state;
ALTER TABLE person_social RENAME TO contact_social;

-- 2. columns (their table already renamed above)
ALTER TABLE activity_link RENAME COLUMN person_id TO contact_id;
ALTER TABLE activity_participant RENAME COLUMN person_id TO contact_id;
ALTER TABLE capture_backfill RENAME COLUMN people_created TO contacts_created;
ALTER TABLE communication_basis RENAME COLUMN person_id TO contact_id;
ALTER TABLE communication_suppression RENAME COLUMN person_id TO contact_id;
ALTER TABLE confirm_token RENAME COLUMN person_id TO contact_id;
ALTER TABLE consent_doi_token RENAME COLUMN person_id TO contact_id;
ALTER TABLE consent_event RENAME COLUMN person_id TO contact_id;
ALTER TABLE consent_existing_customer_flag RENAME COLUMN person_id TO contact_id;
ALTER TABLE consent_qualifying_event RENAME COLUMN person_id TO contact_id;
ALTER TABLE conversation_claim RENAME COLUMN person_id TO contact_id;
ALTER TABLE data_subject_request RENAME COLUMN person_id TO contact_id;
ALTER TABLE dedupe_candidate RENAME COLUMN left_person_id TO left_contact_id;
ALTER TABLE dedupe_candidate RENAME COLUMN right_person_id TO right_contact_id;
ALTER TABLE graph_contact_edge RENAME COLUMN person_a TO contact_a;
ALTER TABLE graph_contact_edge RENAME COLUMN person_b TO contact_b;
ALTER TABLE graph_interaction_edge RENAME COLUMN person_id TO contact_id;
ALTER TABLE intro_request RENAME COLUMN person_id TO contact_id;
ALTER TABLE intro_request RENAME COLUMN through_person_id TO through_contact_id;
ALTER TABLE lead RENAME COLUMN promoted_person_id TO promoted_contact_id;
ALTER TABLE linkedin_connection RENAME COLUMN matched_person_id TO matched_contact_id;
ALTER TABLE contact_acquisition_evidence RENAME COLUMN person_id TO contact_id;
ALTER TABLE contact_brief RENAME COLUMN person_id TO contact_id;
ALTER TABLE contact_channel_identity RENAME COLUMN person_id TO contact_id;
ALTER TABLE contact_confirm_submission RENAME COLUMN person_id TO contact_id;
ALTER TABLE contact_consent RENAME COLUMN person_id TO contact_id;
ALTER TABLE contact_email RENAME COLUMN person_id TO contact_id;
ALTER TABLE contact_moment_dismissal RENAME COLUMN person_id TO contact_id;
ALTER TABLE contact_phone RENAME COLUMN person_id TO contact_id;
ALTER TABLE contact_profile_field RENAME COLUMN person_id TO contact_id;
ALTER TABLE contact_provider_claim RENAME COLUMN person_id TO contact_id;
ALTER TABLE contact_signature_enrich_state RENAME COLUMN person_id TO contact_id;
ALTER TABLE contact_social RENAME COLUMN person_id TO contact_id;
ALTER TABLE preference_token RENAME COLUMN person_email_id TO contact_email_id;
ALTER TABLE preference_token RENAME COLUMN person_id TO contact_id;
ALTER TABLE privacy_notice_case RENAME COLUMN person_id TO contact_id;
ALTER TABLE provider_applied_field RENAME COLUMN person_id TO contact_id;
ALTER TABLE provider_run RENAME COLUMN person_id TO contact_id;
ALTER TABLE relationship RENAME COLUMN counterparty_person_id TO counterparty_contact_id;
ALTER TABLE relationship RENAME COLUMN person_id TO contact_id;
ALTER TABLE relationship_nudge_dismissal RENAME COLUMN person_id TO contact_id;
ALTER TABLE sdr_handoff RENAME COLUMN person_id TO contact_id;
ALTER TABLE signal RENAME COLUMN resolved_person_id TO resolved_contact_id;
ALTER TABLE site_read RENAME COLUMN people TO contacts;
ALTER TABLE withdrawal_credential RENAME COLUMN person_id TO contact_id;

-- 3. constraints
ALTER TABLE activity_link RENAME CONSTRAINT activity_link_person_id_fkey TO activity_link_contact_id_fkey;
ALTER TABLE activity_participant RENAME CONSTRAINT activity_participant_person_fkey TO activity_participant_contact_fkey;
ALTER TABLE communication_basis RENAME CONSTRAINT communication_basis_person_id_fkey TO communication_basis_contact_id_fkey;
ALTER TABLE communication_suppression RENAME CONSTRAINT communication_suppression_person_id_fkey TO communication_suppression_contact_id_fkey;
ALTER TABLE confirm_token RENAME CONSTRAINT confirm_token_person_fkey TO confirm_token_contact_fkey;
ALTER TABLE consent_doi_token RENAME CONSTRAINT consent_doi_token_person_id_fkey TO consent_doi_token_contact_id_fkey;
ALTER TABLE consent_event RENAME CONSTRAINT consent_event_person_id_fkey TO consent_event_contact_id_fkey;
ALTER TABLE consent_existing_customer_flag RENAME CONSTRAINT consent_existing_customer_person_fkey TO consent_existing_customer_contact_fkey;
ALTER TABLE consent_qualifying_event RENAME CONSTRAINT consent_qualifying_event_person_fkey TO consent_qualifying_event_contact_fkey;
ALTER TABLE conversation_claim RENAME CONSTRAINT conversation_claim_person_fkey TO conversation_claim_contact_fkey;
ALTER TABLE data_subject_request RENAME CONSTRAINT data_subject_request_person_id_fkey TO data_subject_request_contact_id_fkey;
ALTER TABLE dedupe_candidate RENAME CONSTRAINT dedupe_candidate_left_person_id_fkey TO dedupe_candidate_left_contact_id_fkey;
ALTER TABLE dedupe_candidate RENAME CONSTRAINT dedupe_candidate_right_person_id_fkey TO dedupe_candidate_right_contact_id_fkey;
ALTER TABLE graph_interaction_edge RENAME CONSTRAINT graph_interaction_edge_workspace_id_person_id_fkey TO graph_interaction_edge_workspace_id_contact_id_fkey;
ALTER TABLE intro_request RENAME CONSTRAINT intro_request_person_id_fkey TO intro_request_contact_id_fkey;
ALTER TABLE intro_request RENAME CONSTRAINT intro_request_through_person_id_fkey TO intro_request_through_contact_id_fkey;
ALTER TABLE lead RENAME CONSTRAINT lead_promoted_person_id_fkey TO lead_promoted_contact_id_fkey;
ALTER TABLE linkedin_connection RENAME CONSTRAINT linkedin_connection_workspace_id_matched_person_id_fkey TO linkedin_connection_workspace_id_matched_contact_id_fkey;
ALTER TABLE contact RENAME CONSTRAINT person_converted_from_lead_id_fkey TO contact_converted_from_lead_id_fkey;
ALTER TABLE contact RENAME CONSTRAINT person_merged_into_id_fkey TO contact_merged_into_id_fkey;
ALTER TABLE contact RENAME CONSTRAINT person_owner_id_fkey TO contact_owner_id_fkey;
ALTER TABLE contact RENAME CONSTRAINT person_owner_private_names_its_owner TO contact_owner_private_names_its_owner;
ALTER TABLE contact RENAME CONSTRAINT person_pkey TO contact_pkey;
ALTER TABLE contact RENAME CONSTRAINT person_visibility_check TO contact_visibility_check;
ALTER TABLE contact_acquisition_evidence RENAME CONSTRAINT person_acquisition_evidence_kind TO contact_acquisition_evidence_kind;
ALTER TABLE contact_acquisition_evidence RENAME CONSTRAINT person_acquisition_evidence_person_id_fkey TO contact_acquisition_evidence_contact_id_fkey;
ALTER TABLE contact_acquisition_evidence RENAME CONSTRAINT person_acquisition_evidence_pkey TO contact_acquisition_evidence_pkey;
ALTER TABLE contact_acquisition_evidence RENAME CONSTRAINT person_acquisition_evidence_source_shape TO contact_acquisition_evidence_source_shape;
ALTER TABLE contact_brief RENAME CONSTRAINT person_brief_generated_by_check TO contact_brief_generated_by_check;
ALTER TABLE contact_brief RENAME CONSTRAINT person_brief_person_fkey TO contact_brief_contact_fkey;
ALTER TABLE contact_brief RENAME CONSTRAINT person_brief_pkey TO contact_brief_pkey;
ALTER TABLE contact_brief RENAME CONSTRAINT person_brief_user_fkey TO contact_brief_user_fkey;
ALTER TABLE contact_channel_identity RENAME CONSTRAINT person_channel_identity_person_id_fkey TO contact_channel_identity_contact_id_fkey;
ALTER TABLE contact_channel_identity RENAME CONSTRAINT person_channel_identity_pkey TO contact_channel_identity_pkey;
ALTER TABLE contact_channel_identity RENAME CONSTRAINT person_channel_identity_provider_fkey TO contact_channel_identity_provider_fkey;
ALTER TABLE contact_confirm_submission RENAME CONSTRAINT person_confirm_submission_field_matches_kind TO contact_confirm_submission_field_matches_kind;
ALTER TABLE contact_confirm_submission RENAME CONSTRAINT person_confirm_submission_kind_check TO contact_confirm_submission_kind_check;
ALTER TABLE contact_confirm_submission RENAME CONSTRAINT person_confirm_submission_person_fkey TO contact_confirm_submission_contact_fkey;
ALTER TABLE contact_confirm_submission RENAME CONSTRAINT person_confirm_submission_pkey TO contact_confirm_submission_pkey;
ALTER TABLE contact_confirm_submission RENAME CONSTRAINT person_confirm_submission_resolution_check TO contact_confirm_submission_resolution_check;
ALTER TABLE contact_confirm_submission RENAME CONSTRAINT person_confirm_submission_resolved_together TO contact_confirm_submission_resolved_together;
ALTER TABLE contact_confirm_submission RENAME CONSTRAINT person_confirm_submission_token_fkey TO contact_confirm_submission_token_fkey;
ALTER TABLE contact_consent RENAME CONSTRAINT person_consent_lead_id_fkey TO contact_consent_lead_id_fkey;
ALTER TABLE contact_consent RENAME CONSTRAINT person_consent_lead_unique TO contact_consent_lead_unique;
ALTER TABLE contact_consent RENAME CONSTRAINT person_consent_person_id_fkey TO contact_consent_contact_id_fkey;
ALTER TABLE contact_consent RENAME CONSTRAINT person_consent_pkey TO contact_consent_pkey;
ALTER TABLE contact_consent RENAME CONSTRAINT person_consent_purpose_id_fkey TO contact_consent_purpose_id_fkey;
ALTER TABLE contact_consent RENAME CONSTRAINT person_consent_state_check TO contact_consent_state_check;
ALTER TABLE contact_consent RENAME CONSTRAINT person_consent_subject TO contact_consent_subject;
ALTER TABLE contact_consent RENAME CONSTRAINT person_consent_unique TO contact_consent_unique;
ALTER TABLE contact_email RENAME CONSTRAINT person_email_email_type_check TO contact_email_email_type_check;
ALTER TABLE contact_email RENAME CONSTRAINT person_email_norm TO contact_email_norm;
ALTER TABLE contact_email RENAME CONSTRAINT person_email_person_id_fkey TO contact_email_contact_id_fkey;
ALTER TABLE contact_email RENAME CONSTRAINT person_email_pkey TO contact_email_pkey;
ALTER TABLE contact_moment_dismissal RENAME CONSTRAINT person_moment_dismissal_person_fkey TO contact_moment_dismissal_contact_fkey;
ALTER TABLE contact_moment_dismissal RENAME CONSTRAINT person_moment_dismissal_pkey TO contact_moment_dismissal_pkey;
ALTER TABLE contact_moment_dismissal RENAME CONSTRAINT person_moment_dismissal_user_fkey TO contact_moment_dismissal_user_fkey;
ALTER TABLE contact_phone RENAME CONSTRAINT person_phone_person_id_fkey TO contact_phone_contact_id_fkey;
ALTER TABLE contact_phone RENAME CONSTRAINT person_phone_phone_type_check TO contact_phone_phone_type_check;
ALTER TABLE contact_phone RENAME CONSTRAINT person_phone_pkey TO contact_phone_pkey;
ALTER TABLE contact_phone RENAME CONSTRAINT person_phone_superseded_phone_id_fkey TO contact_phone_superseded_phone_id_fkey;
ALTER TABLE contact_profile_field RENAME CONSTRAINT person_profile_field_confidence_check TO contact_profile_field_confidence_check;
ALTER TABLE contact_profile_field RENAME CONSTRAINT person_profile_field_field_check TO contact_profile_field_field_check;
ALTER TABLE contact_profile_field RENAME CONSTRAINT person_profile_field_person_fk TO contact_profile_field_contact_fk;
ALTER TABLE contact_profile_field RENAME CONSTRAINT person_profile_field_pkey TO contact_profile_field_pkey;
ALTER TABLE contact_profile_field RENAME CONSTRAINT uq_person_profile_field TO uq_contact_profile_field;
ALTER TABLE contact_provider_claim RENAME CONSTRAINT person_provider_claim_claim_key_check TO contact_provider_claim_claim_key_check;
ALTER TABLE contact_provider_claim RENAME CONSTRAINT person_provider_claim_confidence_check TO contact_provider_claim_confidence_check;
ALTER TABLE contact_provider_claim RENAME CONSTRAINT person_provider_claim_person_id_fkey TO contact_provider_claim_contact_id_fkey;
ALTER TABLE contact_provider_claim RENAME CONSTRAINT person_provider_claim_pkey TO contact_provider_claim_pkey;
ALTER TABLE contact_provider_claim RENAME CONSTRAINT person_provider_claim_run_id_claim_key_key TO contact_provider_claim_run_id_claim_key_key;
ALTER TABLE contact_provider_claim RENAME CONSTRAINT person_provider_claim_run_id_fkey TO contact_provider_claim_run_id_fkey;
ALTER TABLE contact_signature_enrich_state RENAME CONSTRAINT person_signature_enrich_state_activity_id_fkey TO contact_signature_enrich_state_activity_id_fkey;
ALTER TABLE contact_signature_enrich_state RENAME CONSTRAINT person_signature_enrich_state_person_id_fkey TO contact_signature_enrich_state_contact_id_fkey;
ALTER TABLE contact_signature_enrich_state RENAME CONSTRAINT person_signature_enrich_state_pkey TO contact_signature_enrich_state_pkey;
ALTER TABLE contact_social RENAME CONSTRAINT person_social_person_id_fkey TO contact_social_contact_id_fkey;
ALTER TABLE contact_social RENAME CONSTRAINT person_social_person_id_platform_key TO contact_social_contact_id_platform_key;
ALTER TABLE contact_social RENAME CONSTRAINT person_social_pkey TO contact_social_pkey;
ALTER TABLE preference_token RENAME CONSTRAINT preference_token_person_email_id_fkey TO preference_token_contact_email_id_fkey;
ALTER TABLE preference_token RENAME CONSTRAINT preference_token_person_fkey TO preference_token_contact_fkey;
ALTER TABLE privacy_notice_case RENAME CONSTRAINT privacy_notice_case_person_fkey TO privacy_notice_case_contact_fkey;
ALTER TABLE provider_applied_field RENAME CONSTRAINT provider_applied_field_person_id_fkey TO provider_applied_field_contact_id_fkey;
ALTER TABLE provider_run RENAME CONSTRAINT provider_run_person_id_fkey TO provider_run_contact_id_fkey;
ALTER TABLE relationship RENAME CONSTRAINT relationship_counterparty_person_id_fkey TO relationship_counterparty_contact_id_fkey;
ALTER TABLE relationship RENAME CONSTRAINT relationship_person_id_fkey TO relationship_contact_id_fkey;
ALTER TABLE relationship_nudge_dismissal RENAME CONSTRAINT relationship_nudge_dismissal_person_id_fkey TO relationship_nudge_dismissal_contact_id_fkey;
ALTER TABLE sdr_handoff RENAME CONSTRAINT sdr_handoff_person_id_fkey TO sdr_handoff_contact_id_fkey;
ALTER TABLE signal RENAME CONSTRAINT signal_resolved_person_fkey TO signal_resolved_contact_fkey;
ALTER TABLE withdrawal_credential RENAME CONSTRAINT withdrawal_credential_person_id_fkey TO withdrawal_credential_contact_id_fkey;

-- 4. indexes (constraint-backed ones moved with their constraint; the two
-- provider_run ones carry the literal in their PREDICATE and are rebuilt in
-- section 7 instead, because a rename does not reach a predicate)
ALTER INDEX communication_basis_live_person RENAME TO communication_basis_live_contact;
ALTER INDEX communication_suppression_live_person RENAME TO communication_suppression_live_contact;
ALTER INDEX consent_qualifying_event_person_ix RENAME TO consent_qualifying_event_contact_ix;
ALTER INDEX conversation_claim_person_ix RENAME TO conversation_claim_contact_ix;
ALTER INDEX idx_alink_person RENAME TO idx_alink_contact;
ALTER INDEX idx_aparticipant_person RENAME TO idx_aparticipant_contact;
ALTER INDEX idx_consent_doi_token_person RENAME TO idx_consent_doi_token_contact;
ALTER INDEX idx_consent_event_person RENAME TO idx_consent_event_contact;
ALTER INDEX idx_graph_edge_person RENAME TO idx_graph_edge_contact;
ALTER INDEX idx_person_channel_identity_person RENAME TO idx_contact_channel_identity_contact;
ALTER INDEX idx_person_created_keyset RENAME TO idx_contact_created_keyset;
ALTER INDEX idx_person_email_correspondence RENAME TO idx_contact_email_correspondence;
ALTER INDEX idx_person_email_person RENAME TO idx_contact_email_contact;
ALTER INDEX idx_person_from_lead RENAME TO idx_contact_from_lead;
ALTER INDEX idx_person_last_activity_keyset RENAME TO idx_contact_last_activity_keyset;
ALTER INDEX idx_person_merged_into RENAME TO idx_contact_merged_into;
ALTER INDEX idx_person_name_keyset RENAME TO idx_contact_name_keyset;
ALTER INDEX idx_person_name_keyset_desc RENAME TO idx_contact_name_keyset_desc;
ALTER INDEX idx_person_name_trgm RENAME TO idx_contact_name_trgm;
ALTER INDEX idx_person_owner RENAME TO idx_contact_owner;
ALTER INDEX idx_person_phone_person RENAME TO idx_contact_phone_contact;
ALTER INDEX idx_person_profile_field RENAME TO idx_contact_profile_field;
ALTER INDEX idx_person_search RENAME TO idx_contact_search;
ALTER INDEX idx_person_social_person RENAME TO idx_contact_social_contact;
ALTER INDEX idx_person_updated_keyset RENAME TO idx_contact_updated_keyset;
ALTER INDEX idx_rel_employer_people RENAME TO idx_rel_employer_contacts;
ALTER INDEX idx_rel_history_person RENAME TO idx_rel_history_contact;
ALTER INDEX idx_rel_company_people RENAME TO idx_rel_company_contacts;
ALTER INDEX idx_rel_person_companies RENAME TO idx_rel_contact_companies;
ALTER INDEX idx_rel_person_projects RENAME TO idx_rel_contact_projects;
ALTER INDEX idx_rel_traverse_person RENAME TO idx_rel_traverse_contact;
ALTER INDEX idx_rel_works_with_person RENAME TO idx_rel_works_with_contact;
ALTER INDEX idx_sdr_handoff_person RENAME TO idx_sdr_handoff_contact;
ALTER INDEX idx_withdrawal_credential_person RENAME TO idx_withdrawal_credential_contact;
ALTER INDEX intro_request_for_person RENAME TO intro_request_for_contact;
ALTER INDEX ix_confirm_token_person RENAME TO ix_confirm_token_contact;
ALTER INDEX ix_person_confirm_submission_open RENAME TO ix_contact_confirm_submission_open;
ALTER INDEX ix_person_confirm_submission_person RENAME TO ix_contact_confirm_submission_contact;
ALTER INDEX person_acquisition_evidence_by_person RENAME TO contact_acquisition_evidence_by_contact;
ALTER INDEX person_brief_person_ix RENAME TO contact_brief_contact_ix;
ALTER INDEX person_moment_dismissal_person_ix RENAME TO contact_moment_dismissal_contact_ix;
ALTER INDEX person_provider_claim_latest RENAME TO contact_provider_claim_latest;
ALTER INDEX provider_applied_field_by_person RENAME TO provider_applied_field_by_contact;
ALTER INDEX uq_person_channel_identity RENAME TO uq_contact_channel_identity;
ALTER INDEX uq_person_email_dedupe RENAME TO uq_contact_email_dedupe;
ALTER INDEX uq_person_email_primary RENAME TO uq_contact_email_primary;
ALTER INDEX uq_person_phone_primary RENAME TO uq_contact_phone_primary;
ALTER INDEX uq_preference_token_person_address RENAME TO uq_preference_token_contact_address;
ALTER INDEX uq_rel_deal_person_role RENAME TO uq_rel_deal_contact_role;

-- 5. triggers
ALTER TRIGGER trg_person_updated ON contact RENAME TO trg_contact_updated;
ALTER TRIGGER trg_person_channel_identity_updated ON contact_channel_identity RENAME TO trg_contact_channel_identity_updated;
ALTER TRIGGER trg_person_email_updated ON contact_email RENAME TO trg_contact_email_updated;
ALTER TRIGGER trg_person_phone_updated ON contact_phone RENAME TO trg_contact_phone_updated;
ALTER TRIGGER trg_person_profile_field_updated ON contact_profile_field RENAME TO trg_contact_profile_field_updated;

-- 6. the functions that name the old column in their BODY.
--
-- A rename moves a catalog row and leaves function text alone, so a body
-- naming person_id keeps naming it and fails at the next call rather than
-- here. These eight are the ones that do, and they come BEFORE the data
-- below: activity_link_last_activity fires on every UPDATE of that table, so
-- one row moved while a body still names person_id takes the migration down
-- with it. Each is recreated with the same
-- logic and the new word; the two meeting refusals carry it only in the
-- message they raise, and a message that says person for a record called
-- contact is the drift this migration exists to end.
DROP FUNCTION last_activity_of_person(uuid);

CREATE FUNCTION last_activity_of_contact(cid uuid) RETURNS timestamptz
    LANGUAGE sql STABLE
    AS $$
  SELECT max(a.occurred_at)
    FROM activity_link l
    JOIN activity a ON a.id = l.activity_id AND a.archived_at IS NULL
     AND a.audience = 'workspace'
     AND a.origin NOT IN ('system_remediation', 'system_notice')
   WHERE l.contact_id = cid
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
      JOIN activity_link l ON l.contact_id = r.contact_id
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
    WHEN 'contact'::regclass THEN
      PERFORM 1 FROM contact WHERE id = rid FOR UPDATE;
      v := last_activity_of_contact(rid);
      PERFORM set_config('margince.last_activity_move', 'on', true);
      UPDATE contact SET last_activity_at = v WHERE id = rid;
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

-- Recreated rather than replaced: CREATE OR REPLACE cannot rename an input
-- parameter, and `pid` named the person.
CREATE FUNCTION refresh_last_activity_for_link(cid uuid, did uuid, oid uuid) RETURNS void
    LANGUAGE plpgsql
    AS $$
DECLARE
  reached uuid;
BEGIN
  PERFORM move_last_activity('contact', cid);
  PERFORM move_last_activity('deal', did);
  -- Ordered by id: two writers reaching the same accounts lock them in the
  -- same order, so they queue rather than deadlock.
  FOR reached IN
     SELECT x FROM (
       SELECT oid AS x WHERE oid IS NOT NULL
       UNION SELECT d.company_id FROM deal d WHERE d.id = did AND d.company_id IS NOT NULL
       UNION SELECT r.company_id FROM relationship r
              WHERE r.contact_id = cid AND r.kind = 'employment' AND r.ended_at IS NULL AND r.archived_at IS NULL
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
  PERFORM refresh_last_activity_for_link(l.contact_id, l.deal_id, l.company_id)
     FROM activity_link l WHERE l.activity_id = NEW.id;
  RETURN NULL;
END;
$$;

CREATE OR REPLACE FUNCTION trg_activity_link_last_activity() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
  IF TG_OP IN ('DELETE', 'UPDATE') THEN
    PERFORM refresh_last_activity_for_link(OLD.contact_id, OLD.deal_id, OLD.company_id);
  END IF;
  IF TG_OP IN ('INSERT', 'UPDATE') THEN
    PERFORM refresh_last_activity_for_link(NEW.contact_id, NEW.deal_id, NEW.company_id);
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
            'a % is with a contact, not with a company: link it to the contact who was there, and the company sees it through them',
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
            'this activity is linked to a company, so it cannot become a %: a % is with a contact',
            NEW.kind, NEW.kind
            USING ERRCODE = 'check_violation';
    END IF;

    RETURN NEW;
END;
$$;

-- 7. the CHECKs that carry the word as a VALUE, and the rows that store it.
--
-- Each is dropped, the stored values moved, and the constraint re-added
-- naming the new word. 'personal' in capture_pending_counterparty is a
-- different word and stays.

-- activity_link.entity_type
ALTER TABLE activity_link DROP CONSTRAINT activity_link_entity_type_check;
ALTER TABLE activity_link DROP CONSTRAINT activity_link_shape;
UPDATE activity_link SET entity_type = 'contact' WHERE entity_type = 'person';
ALTER TABLE activity_link ADD CONSTRAINT activity_link_entity_type_check
  CHECK (entity_type IN ('contact', 'company', 'deal', 'lead', 'project'));
ALTER TABLE activity_link ADD CONSTRAINT activity_link_shape
  CHECK (((entity_type = 'contact') AND (contact_id IS NOT NULL) AND (company_id IS NULL) AND (deal_id IS NULL) AND (lead_id IS NULL) AND (project_id IS NULL)) OR ((entity_type = 'company') AND (company_id IS NOT NULL) AND (contact_id IS NULL) AND (deal_id IS NULL) AND (lead_id IS NULL) AND (project_id IS NULL)) OR ((entity_type = 'deal') AND (deal_id IS NOT NULL) AND (contact_id IS NULL) AND (company_id IS NULL) AND (lead_id IS NULL) AND (project_id IS NULL)) OR ((entity_type = 'lead') AND (lead_id IS NOT NULL) AND (contact_id IS NULL) AND (company_id IS NULL) AND (deal_id IS NULL) AND (project_id IS NULL)) OR ((entity_type = 'project') AND (project_id IS NOT NULL) AND (contact_id IS NULL) AND (company_id IS NULL) AND (deal_id IS NULL) AND (lead_id IS NULL)));

-- ai_feedback.subject_type
ALTER TABLE ai_feedback DROP CONSTRAINT ai_feedback_subject_type_check;
UPDATE ai_feedback SET subject_type = 'contact' WHERE subject_type = 'person';
ALTER TABLE ai_feedback ADD CONSTRAINT ai_feedback_subject_type_check
  CHECK (subject_type IN ('company', 'contact', 'deal', 'lead'));

-- ai_task_run.quantity_unit — "3 people" is this record type counted, and the
-- screen that renders it says contacts.
ALTER TABLE ai_task_run DROP CONSTRAINT ai_task_run_quantity_unit_check;
UPDATE ai_task_run SET quantity_unit = 'contacts' WHERE quantity_unit = 'people';
ALTER TABLE ai_task_run ADD CONSTRAINT ai_task_run_quantity_unit_check
  CHECK ((quantity_unit IS NULL) OR quantity_unit IN ('messages', 'records', 'contacts', 'documents'));

-- attachment.entity_type
ALTER TABLE attachment DROP CONSTRAINT attachment_entity_type_check;
UPDATE attachment SET entity_type = 'contact' WHERE entity_type = 'person';
ALTER TABLE attachment ADD CONSTRAINT attachment_entity_type_check
  CHECK (entity_type IN ('contact', 'company', 'deal', 'lead', 'activity', 'project', 'relationship', 'partner'));

-- capture_backfill_creation.kind
ALTER TABLE capture_backfill_creation DROP CONSTRAINT capture_backfill_creation_kind;
UPDATE capture_backfill_creation SET kind = 'contact' WHERE kind = 'person';
ALTER TABLE capture_backfill_creation ADD CONSTRAINT capture_backfill_creation_kind
  CHECK (kind IN ('contact', 'company_queued'));

-- capture_pending_counterparty.kind — 'personal' beside 'person' is the reason
-- this constraint is written out rather than swept.
ALTER TABLE capture_pending_counterparty DROP CONSTRAINT capture_pending_counterparty_kind_check;
UPDATE capture_pending_counterparty SET kind = 'contact' WHERE kind = 'person';
ALTER TABLE capture_pending_counterparty ADD CONSTRAINT capture_pending_counterparty_kind_check
  CHECK ((kind IS NULL) OR kind IN ('contact', 'role_mailbox', 'company_sender', 'newsletter', 'transactional', 'spam', 'personal', 'advisor'));

-- communication_decision.subject_kind
ALTER TABLE communication_decision DROP CONSTRAINT communication_decision_subject_kind;
UPDATE communication_decision SET subject_kind = 'contact' WHERE subject_kind = 'person';
ALTER TABLE communication_decision ADD CONSTRAINT communication_decision_subject_kind
  CHECK ((subject_kind IS NULL) OR subject_kind IN ('contact', 'lead'));

-- custom_field.object
ALTER TABLE custom_field DROP CONSTRAINT custom_field_object_check;
UPDATE custom_field SET object = 'contact' WHERE object = 'person';
ALTER TABLE custom_field ADD CONSTRAINT custom_field_object_check
  CHECK (object IN ('contact', 'company', 'deal', 'lead', 'activity', 'project', 'relationship', 'partner'));

-- dedupe_candidate.entity_type
ALTER TABLE dedupe_candidate DROP CONSTRAINT dedupe_candidate_entity_type_check;
ALTER TABLE dedupe_candidate DROP CONSTRAINT dedupe_candidate_shape;
UPDATE dedupe_candidate SET entity_type = 'contact' WHERE entity_type = 'person';
ALTER TABLE dedupe_candidate ADD CONSTRAINT dedupe_candidate_entity_type_check
  CHECK (entity_type IN ('contact', 'company', 'lead'));
ALTER TABLE dedupe_candidate ADD CONSTRAINT dedupe_candidate_shape
  CHECK (((entity_type = 'contact') AND (left_contact_id IS NOT NULL) AND (right_contact_id IS NOT NULL) AND (left_company_id IS NULL) AND (right_company_id IS NULL) AND (left_lead_id IS NULL) AND (right_lead_id IS NULL)) OR ((entity_type = 'company') AND (left_company_id IS NOT NULL) AND (right_company_id IS NOT NULL) AND (left_contact_id IS NULL) AND (right_contact_id IS NULL) AND (left_lead_id IS NULL) AND (right_lead_id IS NULL)) OR ((entity_type = 'lead') AND (left_lead_id IS NOT NULL) AND (right_lead_id IS NOT NULL) AND (left_contact_id IS NULL) AND (right_contact_id IS NULL) AND (left_company_id IS NULL) AND (right_company_id IS NULL)));

-- embedding.entity_type
ALTER TABLE embedding DROP CONSTRAINT embedding_entity_type_check;
UPDATE embedding SET entity_type = 'contact' WHERE entity_type = 'person';
ALTER TABLE embedding ADD CONSTRAINT embedding_entity_type_check
  CHECK (entity_type IN ('contact', 'company', 'deal', 'lead', 'activity', 'project', 'relationship', 'partner'));

-- field_provenance.object_type
ALTER TABLE field_provenance DROP CONSTRAINT field_provenance_object_type_check;
UPDATE field_provenance SET object_type = 'contact' WHERE object_type = 'person';
ALTER TABLE field_provenance ADD CONSTRAINT field_provenance_object_type_check
  CHECK (object_type IN ('contact', 'company', 'deal', 'lead', 'activity', 'project', 'relationship', 'partner'));

-- list.entity_type
ALTER TABLE list DROP CONSTRAINT list_entity_type_check;
UPDATE list SET entity_type = 'contact' WHERE entity_type = 'person';
ALTER TABLE list ADD CONSTRAINT list_entity_type_check
  CHECK (entity_type IN ('contact', 'company', 'deal', 'lead', 'project'));

-- list_member.entity_type
ALTER TABLE list_member DROP CONSTRAINT list_member_entity_type_check;
UPDATE list_member SET entity_type = 'contact' WHERE entity_type = 'person';
ALTER TABLE list_member ADD CONSTRAINT list_member_entity_type_check
  CHECK (entity_type IN ('contact', 'company', 'deal', 'lead', 'project'));

-- provider_applied_field.target_table — the values ARE table names, so they
-- move with the tables.
ALTER TABLE provider_applied_field DROP CONSTRAINT provider_applied_field_target_check;
UPDATE provider_applied_field SET target_table = 'contact' WHERE target_table = 'person';
UPDATE provider_applied_field SET target_table = 'contact_social' WHERE target_table = 'person_social';
UPDATE provider_applied_field SET target_table = 'contact_email' WHERE target_table = 'person_email';
UPDATE provider_applied_field SET target_table = 'contact_phone' WHERE target_table = 'person_phone';
ALTER TABLE provider_applied_field ADD CONSTRAINT provider_applied_field_target_check
  CHECK (target_table IN ('contact', 'contact_social', 'contact_email', 'contact_phone', 'relationship'));

-- provider_run.subject_kind. Its two partial indexes carry the literal in
-- their PREDICATE, which a rename does not reach, so they are rebuilt.
DROP INDEX provider_run_one_live_person_fingerprint;
DROP INDEX provider_run_person_history;
ALTER TABLE provider_run DROP CONSTRAINT provider_run_subject_kind_check;
ALTER TABLE provider_run DROP CONSTRAINT provider_run_subject_shape;
UPDATE provider_run SET subject_kind = 'contact' WHERE subject_kind = 'person';
ALTER TABLE provider_run ADD CONSTRAINT provider_run_subject_kind_check
  CHECK (subject_kind IN ('contact', 'scrubbed'));
ALTER TABLE provider_run ADD CONSTRAINT provider_run_subject_shape
  CHECK (((subject_kind = 'contact') AND (contact_id IS NOT NULL)) OR ((subject_kind = 'scrubbed') AND (contact_id IS NULL)));
CREATE UNIQUE INDEX provider_run_one_live_contact_fingerprint ON provider_run (contact_id, provider, input_fingerprint)
  WHERE ((subject_kind = 'contact') AND (state IN ('queued', 'submitting', 'in_progress', 'submission_unknown')));
CREATE INDEX provider_run_contact_history ON provider_run (contact_id, provider, created_at DESC)
  WHERE (subject_kind = 'contact');

-- record_grant.record_type
ALTER TABLE record_grant DROP CONSTRAINT record_grant_record_type_check;
UPDATE record_grant SET record_type = 'contact' WHERE record_type = 'person';
ALTER TABLE record_grant ADD CONSTRAINT record_grant_record_type_check
  CHECK (record_type IN ('contact', 'company', 'deal', 'lead', 'project'));

-- saved_view.resource — the plural, which is the collection a view is over.
ALTER TABLE saved_view DROP CONSTRAINT saved_view_resource_check;
UPDATE saved_view SET resource = 'contacts' WHERE resource = 'people';
ALTER TABLE saved_view ADD CONSTRAINT saved_view_resource_check
  CHECK (resource IN ('contacts', 'companies', 'deals', 'activities', 'leads', 'partners', 'projects'));

-- signal.entity_type
ALTER TABLE signal DROP CONSTRAINT signal_entity_type_check;
UPDATE signal SET entity_type = 'contact' WHERE entity_type = 'person';
ALTER TABLE signal ADD CONSTRAINT signal_entity_type_check
  CHECK ((entity_type IS NULL) OR entity_type IN ('deal', 'company', 'contact', 'project'));

-- taggable.entity_type
ALTER TABLE taggable DROP CONSTRAINT taggable_entity_type_check;
UPDATE taggable SET entity_type = 'contact' WHERE entity_type = 'person';
ALTER TABLE taggable ADD CONSTRAINT taggable_entity_type_check
  CHECK (entity_type IN ('contact', 'company', 'deal', 'lead', 'project'));

-- user_record_view.entity_type
ALTER TABLE user_record_view DROP CONSTRAINT user_record_view_entity_type_check;
UPDATE user_record_view SET entity_type = 'contact' WHERE entity_type = 'person';
ALTER TABLE user_record_view ADD CONSTRAINT user_record_view_entity_type_check
  CHECK (entity_type IN ('company', 'contact'));

-- weekly_plan_commitment.linked_record_type
ALTER TABLE weekly_plan_commitment DROP CONSTRAINT weekly_plan_commitment_link_type_check;
UPDATE weekly_plan_commitment SET linked_record_type = 'contact' WHERE linked_record_type = 'person';
ALTER TABLE weekly_plan_commitment ADD CONSTRAINT weekly_plan_commitment_link_type_check
  CHECK ((linked_record_type IS NULL) OR linked_record_type IN ('deal', 'lead', 'contact', 'company', 'project'));

-- 8. the vocabularies stored WITHOUT a CHECK to enumerate them.
--
-- Section 7 found the stored word wherever a CHECK named it. These columns
-- carry the same word under an OPEN vocabulary, so nothing in the schema says
-- 'contact' is now among the values and nothing would fail until a grant
-- stopped matching a route or a subscription went quiet.

-- A role's permissions are keyed by object name, and `contact:read` is the key
-- every contact route checks.
UPDATE role
   SET permissions = jsonb_set(permissions, '{objects}',
         ((permissions -> 'objects') - 'person')
         || jsonb_build_object('contact', permissions -> 'objects' -> 'person'))
 WHERE permissions -> 'objects' ? 'person';

UPDATE field_mask SET object = 'contact' WHERE object = 'person';

-- An approval names the record its staged actions act on. These are PENDING
-- work: a target left under the old word is an approval whose owner nothing
-- resolves, so it sits unappliable with nothing reporting it.
UPDATE approval SET target_entity_type = 'contact' WHERE target_entity_type = 'person';
UPDATE approval SET co_target_entity_type = 'contact' WHERE co_target_entity_type = 'person';

-- A delivery row carries the record it was about and the event it carried.
-- Retries read both, and the history is what an operator reconciles against.
UPDATE webhook_delivery SET entity_type = 'contact' WHERE entity_type = 'person';
UPDATE webhook_delivery
   SET event_type = 'contact.' || substring(event_type from 8)
 WHERE event_type LIKE 'person.%';

-- An outbox row that has not been relayed yet still names the stream it will
-- be published to, and its envelope names both the event and the record. A
-- published row is history and is left alone; an unpublished one is about to
-- become a message nobody is subscribed to.
UPDATE event_outbox
   SET stream = 'contact.' || substring(stream from 8),
       envelope = jsonb_set(envelope, '{type}',
         to_jsonb('contact.' || substring(envelope ->> 'type' from 8)))
 WHERE published_at IS NULL AND stream LIKE 'person.%';
UPDATE event_outbox
   SET envelope = jsonb_set(envelope, '{entity,type}', '"contact"')
 WHERE published_at IS NULL AND envelope #>> '{entity,type}' = 'person';

-- A subscription names the streams it wants, and the relay now publishes
-- contact.*. Left alone, a subscription would go quiet with nothing reporting it.
UPDATE webhook_subscription
   SET event_types = (
         SELECT array_agg(replace(t, 'person.', 'contact.') ORDER BY t)
           FROM unnest(event_types) AS t
       )
 WHERE EXISTS (SELECT 1 FROM unnest(event_types) AS t WHERE t LIKE 'person.%');

-- A suppression carried over from a consent record names where it came from,
-- and the backfill that reads the provenance back matches on the whole string.
UPDATE communication_suppression
   SET source = 'carried_from_contact_consent'
 WHERE source = 'carried_from_person_consent';
