-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '3s';

-- A row naming an extension's kind cannot satisfy a narrow check, so it goes
-- first. What a link, tag or list held lives on only in the audit log.
DELETE FROM activity_link WHERE entity_type LIKE 'ext:%';
DELETE FROM attachment WHERE entity_type LIKE 'ext:%';
DELETE FROM embedding WHERE entity_type LIKE 'ext:%';
DELETE FROM field_provenance WHERE object_type LIKE 'ext:%';
DELETE FROM list_member_event WHERE entity_type LIKE 'ext:%';
DELETE FROM list_live_member WHERE entity_type LIKE 'ext:%';
DELETE FROM list_member WHERE entity_type LIKE 'ext:%';
DELETE FROM list WHERE entity_type LIKE 'ext:%';
DELETE FROM taggable WHERE entity_type LIKE 'ext:%';
DELETE FROM record_grant WHERE record_type LIKE 'ext:%';
DELETE FROM dedupe_candidate WHERE entity_type LIKE 'ext:%';
DELETE FROM user_record_view WHERE entity_type LIKE 'ext:%';
DELETE FROM signal WHERE entity_type LIKE 'ext:%';
DELETE FROM saved_view WHERE resource LIKE 'ext:%';

DROP INDEX uq_dedupe_candidate_pair_ext;
ALTER TABLE dedupe_candidate DROP CONSTRAINT dedupe_candidate_ext_shape;
ALTER TABLE dedupe_candidate DROP CONSTRAINT dedupe_candidate_shape;
ALTER TABLE dedupe_candidate DROP CONSTRAINT dedupe_candidate_entity_type_check;
ALTER TABLE dedupe_candidate DROP COLUMN left_ext_id, DROP COLUMN right_ext_id;
ALTER TABLE dedupe_candidate
    ADD CONSTRAINT dedupe_candidate_entity_type_check CHECK (
        entity_type IN ('contact', 'company', 'lead')
    ) NOT VALID,
    ADD CONSTRAINT dedupe_candidate_shape CHECK (
        (entity_type = 'contact' AND left_contact_id IS NOT NULL AND right_contact_id IS NOT NULL AND left_company_id IS NULL AND right_company_id IS NULL AND left_lead_id IS NULL AND right_lead_id IS NULL)
        OR (entity_type = 'company' AND left_company_id IS NOT NULL AND right_company_id IS NOT NULL AND left_contact_id IS NULL AND right_contact_id IS NULL AND left_lead_id IS NULL AND right_lead_id IS NULL)
        OR (entity_type = 'lead' AND left_lead_id IS NOT NULL AND right_lead_id IS NOT NULL AND left_contact_id IS NULL AND right_contact_id IS NULL AND left_company_id IS NULL AND right_company_id IS NULL)
    ) NOT VALID;

ALTER TABLE record_grant DROP CONSTRAINT record_grant_record_shape;
ALTER TABLE record_grant DROP CONSTRAINT record_grant_record_type_check;
ALTER TABLE record_grant
    ADD CONSTRAINT record_grant_record_type_check CHECK (
        record_type IN ('contact', 'company', 'deal', 'lead', 'project')
    ) NOT VALID,
    ADD CONSTRAINT record_grant_record_shape CHECK (
        num_nonnulls(contact_id, company_id, deal_id, lead_id, project_id) = 1
    ) NOT VALID;

DROP INDEX uq_activity_link_ext;
ALTER TABLE activity_link DROP CONSTRAINT activity_link_ext_id_matches_type;
ALTER TABLE activity_link DROP CONSTRAINT activity_link_shape;
ALTER TABLE activity_link DROP CONSTRAINT activity_link_entity_type_check;
ALTER TABLE activity_link DROP COLUMN ext_entity_id;
ALTER TABLE activity_link
    ADD CONSTRAINT activity_link_entity_type_check CHECK (
        entity_type IN ('contact', 'company', 'deal', 'lead', 'project')
    ) NOT VALID,
    ADD CONSTRAINT activity_link_shape CHECK (
        (entity_type = 'contact' AND contact_id IS NOT NULL AND company_id IS NULL AND deal_id IS NULL AND lead_id IS NULL AND project_id IS NULL)
        OR (entity_type = 'company' AND company_id IS NOT NULL AND contact_id IS NULL AND deal_id IS NULL AND lead_id IS NULL AND project_id IS NULL)
        OR (entity_type = 'deal' AND deal_id IS NOT NULL AND contact_id IS NULL AND company_id IS NULL AND lead_id IS NULL AND project_id IS NULL)
        OR (entity_type = 'lead' AND lead_id IS NOT NULL AND contact_id IS NULL AND company_id IS NULL AND deal_id IS NULL AND project_id IS NULL)
        OR (entity_type = 'project' AND project_id IS NOT NULL AND contact_id IS NULL AND company_id IS NULL AND deal_id IS NULL AND lead_id IS NULL)
    ) NOT VALID;

ALTER TABLE attachment DROP CONSTRAINT attachment_entity_type_check;
ALTER TABLE attachment
    ADD CONSTRAINT attachment_entity_type_check CHECK (
        entity_type IN ('contact', 'company', 'deal', 'lead', 'activity', 'project', 'relationship', 'partner', 'product', 'offer_template', 'offer', 'commission', 'deal_room', 'deal_room_thread', 'deal_room_comment', 'deal_room_document', 'deal_room_participant')
    ) NOT VALID;

ALTER TABLE embedding DROP CONSTRAINT embedding_entity_type_check;
ALTER TABLE embedding
    ADD CONSTRAINT embedding_entity_type_check CHECK (
        entity_type IN ('contact', 'company', 'deal', 'lead', 'activity', 'project', 'relationship', 'partner', 'product', 'offer_template', 'offer', 'commission', 'deal_room', 'deal_room_thread', 'deal_room_comment', 'deal_room_document', 'deal_room_participant')
    ) NOT VALID;

ALTER TABLE field_provenance DROP CONSTRAINT field_provenance_object_type_check;
ALTER TABLE field_provenance
    ADD CONSTRAINT field_provenance_object_type_check CHECK (
        object_type IN ('contact', 'company', 'deal', 'lead', 'activity', 'project', 'relationship', 'partner', 'product', 'offer_template', 'offer', 'commission', 'deal_room', 'deal_room_thread', 'deal_room_comment', 'deal_room_document', 'deal_room_participant')
    ) NOT VALID;

ALTER TABLE list DROP CONSTRAINT list_entity_type_check;
ALTER TABLE list
    ADD CONSTRAINT list_entity_type_check CHECK (
        entity_type IN ('contact', 'company', 'deal', 'lead', 'project')
    ) NOT VALID;

ALTER TABLE list_member DROP CONSTRAINT list_member_entity_type_check;
ALTER TABLE list_member
    ADD CONSTRAINT list_member_entity_type_check CHECK (
        entity_type IN ('contact', 'company', 'deal', 'lead', 'project')
    ) NOT VALID;

ALTER TABLE list_live_member DROP CONSTRAINT list_live_member_entity_type_check;
ALTER TABLE list_live_member
    ADD CONSTRAINT list_live_member_entity_type_check CHECK (
        entity_type IN ('contact', 'company', 'deal', 'lead', 'project')
    ) NOT VALID;

ALTER TABLE list_member_event DROP CONSTRAINT list_member_event_entity_type_check;
ALTER TABLE list_member_event
    ADD CONSTRAINT list_member_event_entity_type_check CHECK (
        entity_type IN ('contact', 'company', 'deal', 'lead', 'project')
    ) NOT VALID;

ALTER TABLE taggable DROP CONSTRAINT taggable_entity_type_check;
ALTER TABLE taggable
    ADD CONSTRAINT taggable_entity_type_check CHECK (
        entity_type IN ('contact', 'company', 'deal', 'lead', 'project')
    ) NOT VALID;

ALTER TABLE user_record_view DROP CONSTRAINT user_record_view_entity_type_check;
ALTER TABLE user_record_view
    ADD CONSTRAINT user_record_view_entity_type_check CHECK (
        entity_type IN ('company', 'contact')
    ) NOT VALID;

ALTER TABLE signal DROP CONSTRAINT signal_entity_type_check;
ALTER TABLE signal
    ADD CONSTRAINT signal_entity_type_check CHECK (
        entity_type IS NULL OR entity_type IN ('deal', 'company', 'contact', 'project')
    ) NOT VALID;

ALTER TABLE saved_view DROP CONSTRAINT saved_view_resource_check;
ALTER TABLE saved_view
    ADD CONSTRAINT saved_view_resource_check CHECK (
        resource IN ('contacts', 'companies', 'deals', 'activities', 'leads', 'partners', 'projects')
    ) NOT VALID;
