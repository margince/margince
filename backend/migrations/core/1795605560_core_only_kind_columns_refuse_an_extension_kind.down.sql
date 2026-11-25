-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- The six columns accept an extension kind again, and dedupe_candidate gets its
-- extension branch back, empty. No row held an extension kind, so none is lost.
SET LOCAL lock_timeout = '3s';

ALTER TABLE attachment DROP CONSTRAINT IF EXISTS attachment_entity_type_check;
ALTER TABLE attachment
    ADD CONSTRAINT attachment_entity_type_check CHECK (
        entity_type IN ('contact', 'company', 'deal', 'lead', 'activity', 'project', 'relationship', 'partner', 'product', 'offer_template', 'offer', 'commission', 'deal_room', 'deal_room_thread', 'deal_room_comment', 'deal_room_document', 'deal_room_participant')
        OR entity_type ~ '^ext:[a-z0-9]+(-[a-z0-9]+)*:[a-z][a-z0-9_]{0,31}$'
    ) NOT VALID;

ALTER TABLE embedding DROP CONSTRAINT IF EXISTS embedding_entity_type_check;
ALTER TABLE embedding
    ADD CONSTRAINT embedding_entity_type_check CHECK (
        entity_type IN ('contact', 'company', 'deal', 'lead', 'activity', 'project', 'relationship', 'partner', 'product', 'offer_template', 'offer', 'commission', 'deal_room', 'deal_room_thread', 'deal_room_comment', 'deal_room_document', 'deal_room_participant')
        OR entity_type ~ '^ext:[a-z0-9]+(-[a-z0-9]+)*:[a-z][a-z0-9_]{0,31}$'
    ) NOT VALID;

ALTER TABLE field_provenance DROP CONSTRAINT IF EXISTS field_provenance_object_type_check;
ALTER TABLE field_provenance
    ADD CONSTRAINT field_provenance_object_type_check CHECK (
        object_type IN ('contact', 'company', 'deal', 'lead', 'activity', 'project', 'relationship', 'partner', 'product', 'offer_template', 'offer', 'commission', 'deal_room', 'deal_room_thread', 'deal_room_comment', 'deal_room_document', 'deal_room_participant')
        OR object_type ~ '^ext:[a-z0-9]+(-[a-z0-9]+)*:[a-z][a-z0-9_]{0,31}$'
    ) NOT VALID;

ALTER TABLE user_record_view DROP CONSTRAINT IF EXISTS user_record_view_entity_type_check;
ALTER TABLE user_record_view
    ADD CONSTRAINT user_record_view_entity_type_check CHECK (
        entity_type IN ('company', 'contact')
        OR entity_type ~ '^ext:[a-z0-9]+(-[a-z0-9]+)*:[a-z][a-z0-9_]{0,31}$'
    ) NOT VALID;

ALTER TABLE signal DROP CONSTRAINT IF EXISTS signal_entity_type_check;
ALTER TABLE signal
    ADD CONSTRAINT signal_entity_type_check CHECK (
        entity_type IS NULL
        OR entity_type IN ('deal', 'company', 'contact', 'project')
        OR entity_type ~ '^ext:[a-z0-9]+(-[a-z0-9]+)*:[a-z][a-z0-9_]{0,31}$'
    ) NOT VALID;

ALTER TABLE dedupe_candidate DROP CONSTRAINT IF EXISTS dedupe_candidate_entity_type_check;
ALTER TABLE dedupe_candidate DROP CONSTRAINT IF EXISTS dedupe_candidate_shape;
ALTER TABLE dedupe_candidate ADD COLUMN IF NOT EXISTS left_ext_id uuid, ADD COLUMN IF NOT EXISTS right_ext_id uuid;
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
CREATE UNIQUE INDEX IF NOT EXISTS uq_dedupe_candidate_pair_ext ON dedupe_candidate (entity_type, left_ext_id, right_ext_id)
    WHERE left_ext_id IS NOT NULL;
