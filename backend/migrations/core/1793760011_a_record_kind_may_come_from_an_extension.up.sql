-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- A record kind is one of core's words or `ext:<unit>:<kind>`, the form a kind an
-- extension declares takes. Which `<unit>:<kind>` pairs exist is the composed
-- set's to say, so the database holds only the shape and the API holds the set.
--
-- Twelve tables restated a closed list of kinds, and so did the three list
-- tables that travel with them. Each CHECK keeps its core words and gains the
-- pattern. The pattern is the unit name grammar, then `:`, then a lower-case
-- letter and up to 31 more of [a-z0-9_]. No table gains a foreign key to an
-- extension table: an extension role cannot be referenced from core, so a link
-- to an extension record is an id and a kind, and core checks the row through
-- the kind's declared table.
--
-- The CHECKs are NOT VALID, and the next migration validates them. The
-- statements that change a table take ACCESS EXCLUSIVE on a table this
-- migration did not create, so the wait for it is bounded.
SET LOCAL lock_timeout = '3s';

ALTER TABLE activity_link DROP CONSTRAINT activity_link_entity_type_check;
ALTER TABLE activity_link DROP CONSTRAINT activity_link_shape;
ALTER TABLE activity_link ADD COLUMN ext_entity_id uuid;
ALTER TABLE activity_link
    ADD CONSTRAINT activity_link_entity_type_check CHECK (
        entity_type IN ('contact', 'company', 'deal', 'lead', 'project')
        OR entity_type ~ '^ext:[a-z0-9]+(-[a-z0-9]+)*:[a-z][a-z0-9_]{0,31}$'
    ) NOT VALID,
    ADD CONSTRAINT activity_link_shape CHECK (
        (entity_type ~ '^ext:' AND contact_id IS NULL AND company_id IS NULL AND deal_id IS NULL AND lead_id IS NULL AND project_id IS NULL)
        OR (entity_type = 'contact' AND contact_id IS NOT NULL AND company_id IS NULL AND deal_id IS NULL AND lead_id IS NULL AND project_id IS NULL)
        OR (entity_type = 'company' AND company_id IS NOT NULL AND contact_id IS NULL AND deal_id IS NULL AND lead_id IS NULL AND project_id IS NULL)
        OR (entity_type = 'deal' AND deal_id IS NOT NULL AND contact_id IS NULL AND company_id IS NULL AND lead_id IS NULL AND project_id IS NULL)
        OR (entity_type = 'lead' AND lead_id IS NOT NULL AND contact_id IS NULL AND company_id IS NULL AND deal_id IS NULL AND project_id IS NULL)
        OR (entity_type = 'project' AND project_id IS NOT NULL AND contact_id IS NULL AND company_id IS NULL AND deal_id IS NULL AND lead_id IS NULL)
    ) NOT VALID,
    ADD CONSTRAINT activity_link_ext_id_matches_type CHECK (
        (entity_type ~ '^ext:') = (ext_entity_id IS NOT NULL)
    ) NOT VALID;
CREATE UNIQUE INDEX uq_activity_link_ext ON activity_link (activity_id, entity_type, ext_entity_id)
    WHERE ext_entity_id IS NOT NULL;

ALTER TABLE attachment DROP CONSTRAINT attachment_entity_type_check;
ALTER TABLE attachment
    ADD CONSTRAINT attachment_entity_type_check CHECK (
        entity_type IN ('contact', 'company', 'deal', 'lead', 'activity', 'project', 'relationship', 'partner', 'product', 'offer_template', 'offer', 'commission', 'deal_room', 'deal_room_thread', 'deal_room_comment', 'deal_room_document', 'deal_room_participant')
        OR entity_type ~ '^ext:[a-z0-9]+(-[a-z0-9]+)*:[a-z][a-z0-9_]{0,31}$'
    ) NOT VALID;

ALTER TABLE embedding DROP CONSTRAINT embedding_entity_type_check;
ALTER TABLE embedding
    ADD CONSTRAINT embedding_entity_type_check CHECK (
        entity_type IN ('contact', 'company', 'deal', 'lead', 'activity', 'project', 'relationship', 'partner', 'product', 'offer_template', 'offer', 'commission', 'deal_room', 'deal_room_thread', 'deal_room_comment', 'deal_room_document', 'deal_room_participant')
        OR entity_type ~ '^ext:[a-z0-9]+(-[a-z0-9]+)*:[a-z][a-z0-9_]{0,31}$'
    ) NOT VALID;

ALTER TABLE field_provenance DROP CONSTRAINT field_provenance_object_type_check;
ALTER TABLE field_provenance
    ADD CONSTRAINT field_provenance_object_type_check CHECK (
        object_type IN ('contact', 'company', 'deal', 'lead', 'activity', 'project', 'relationship', 'partner', 'product', 'offer_template', 'offer', 'commission', 'deal_room', 'deal_room_thread', 'deal_room_comment', 'deal_room_document', 'deal_room_participant')
        OR object_type ~ '^ext:[a-z0-9]+(-[a-z0-9]+)*:[a-z][a-z0-9_]{0,31}$'
    ) NOT VALID;

ALTER TABLE list DROP CONSTRAINT list_entity_type_check;
ALTER TABLE list
    ADD CONSTRAINT list_entity_type_check CHECK (
        entity_type IN ('contact', 'company', 'deal', 'lead', 'project')
        OR entity_type ~ '^ext:[a-z0-9]+(-[a-z0-9]+)*:[a-z][a-z0-9_]{0,31}$'
    ) NOT VALID;

ALTER TABLE list_member DROP CONSTRAINT list_member_entity_type_check;
ALTER TABLE list_member
    ADD CONSTRAINT list_member_entity_type_check CHECK (
        entity_type IN ('contact', 'company', 'deal', 'lead', 'project')
        OR entity_type ~ '^ext:[a-z0-9]+(-[a-z0-9]+)*:[a-z][a-z0-9_]{0,31}$'
    ) NOT VALID;

ALTER TABLE list_live_member DROP CONSTRAINT list_live_member_entity_type_check;
ALTER TABLE list_live_member
    ADD CONSTRAINT list_live_member_entity_type_check CHECK (
        entity_type IN ('contact', 'company', 'deal', 'lead', 'project')
        OR entity_type ~ '^ext:[a-z0-9]+(-[a-z0-9]+)*:[a-z][a-z0-9_]{0,31}$'
    ) NOT VALID;

ALTER TABLE list_member_event DROP CONSTRAINT list_member_event_entity_type_check;
ALTER TABLE list_member_event
    ADD CONSTRAINT list_member_event_entity_type_check CHECK (
        entity_type IN ('contact', 'company', 'deal', 'lead', 'project')
        OR entity_type ~ '^ext:[a-z0-9]+(-[a-z0-9]+)*:[a-z][a-z0-9_]{0,31}$'
    ) NOT VALID;

ALTER TABLE taggable DROP CONSTRAINT taggable_entity_type_check;
ALTER TABLE taggable
    ADD CONSTRAINT taggable_entity_type_check CHECK (
        entity_type IN ('contact', 'company', 'deal', 'lead', 'project')
        OR entity_type ~ '^ext:[a-z0-9]+(-[a-z0-9]+)*:[a-z][a-z0-9_]{0,31}$'
    ) NOT VALID;

ALTER TABLE record_grant DROP CONSTRAINT record_grant_record_type_check;
ALTER TABLE record_grant DROP CONSTRAINT record_grant_record_shape;
ALTER TABLE record_grant
    ADD CONSTRAINT record_grant_record_type_check CHECK (
        record_type IN ('contact', 'company', 'deal', 'lead', 'project')
        OR record_type ~ '^ext:[a-z0-9]+(-[a-z0-9]+)*:[a-z][a-z0-9_]{0,31}$'
    ) NOT VALID,
    ADD CONSTRAINT record_grant_record_shape CHECK (
        num_nonnulls(contact_id, company_id, deal_id, lead_id, project_id) = CASE WHEN record_type ~ '^ext:' THEN 0 ELSE 1 END
    ) NOT VALID;

ALTER TABLE dedupe_candidate DROP CONSTRAINT dedupe_candidate_entity_type_check;
ALTER TABLE dedupe_candidate DROP CONSTRAINT dedupe_candidate_shape;
ALTER TABLE dedupe_candidate ADD COLUMN left_ext_id uuid, ADD COLUMN right_ext_id uuid;
ALTER TABLE dedupe_candidate
    ADD CONSTRAINT dedupe_candidate_entity_type_check CHECK (
        entity_type IN ('contact', 'company', 'lead')
        OR entity_type ~ '^ext:[a-z0-9]+(-[a-z0-9]+)*:[a-z][a-z0-9_]{0,31}$'
    ) NOT VALID,
    ADD CONSTRAINT dedupe_candidate_shape CHECK (
        (entity_type ~ '^ext:' AND left_contact_id IS NULL AND right_contact_id IS NULL AND left_company_id IS NULL AND right_company_id IS NULL AND left_lead_id IS NULL AND right_lead_id IS NULL)
        OR (entity_type = 'contact' AND left_contact_id IS NOT NULL AND right_contact_id IS NOT NULL AND left_company_id IS NULL AND right_company_id IS NULL AND left_lead_id IS NULL AND right_lead_id IS NULL)
        OR (entity_type = 'company' AND left_company_id IS NOT NULL AND right_company_id IS NOT NULL AND left_contact_id IS NULL AND right_contact_id IS NULL AND left_lead_id IS NULL AND right_lead_id IS NULL)
        OR (entity_type = 'lead' AND left_lead_id IS NOT NULL AND right_lead_id IS NOT NULL AND left_contact_id IS NULL AND right_contact_id IS NULL AND left_company_id IS NULL AND right_company_id IS NULL)
    ) NOT VALID,
    ADD CONSTRAINT dedupe_candidate_ext_shape CHECK (
        CASE WHEN entity_type ~ '^ext:'
             THEN left_ext_id IS NOT NULL AND right_ext_id IS NOT NULL AND left_ext_id < right_ext_id
             ELSE left_ext_id IS NULL AND right_ext_id IS NULL END
    ) NOT VALID;
CREATE UNIQUE INDEX uq_dedupe_candidate_pair_ext ON dedupe_candidate (entity_type, left_ext_id, right_ext_id)
    WHERE left_ext_id IS NOT NULL;

ALTER TABLE user_record_view DROP CONSTRAINT user_record_view_entity_type_check;
ALTER TABLE user_record_view
    ADD CONSTRAINT user_record_view_entity_type_check CHECK (
        entity_type IN ('company', 'contact')
        OR entity_type ~ '^ext:[a-z0-9]+(-[a-z0-9]+)*:[a-z][a-z0-9_]{0,31}$'
    ) NOT VALID;

ALTER TABLE signal DROP CONSTRAINT signal_entity_type_check;
ALTER TABLE signal
    ADD CONSTRAINT signal_entity_type_check CHECK (
        entity_type IS NULL
        OR entity_type IN ('deal', 'company', 'contact', 'project')
        OR entity_type ~ '^ext:[a-z0-9]+(-[a-z0-9]+)*:[a-z][a-z0-9_]{0,31}$'
    ) NOT VALID;

ALTER TABLE saved_view DROP CONSTRAINT saved_view_resource_check;
ALTER TABLE saved_view
    ADD CONSTRAINT saved_view_resource_check CHECK (
        resource IN ('contacts', 'companies', 'deals', 'activities', 'leads', 'partners', 'projects')
        OR resource ~ '^ext:[a-z0-9]+(-[a-z0-9]+)*:[a-z][a-z0-9_]{0,31}$'
    ) NOT VALID;
