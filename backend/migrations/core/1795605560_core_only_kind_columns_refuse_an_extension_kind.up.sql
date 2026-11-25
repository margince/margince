-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- Six columns that name a kind of record go back to core's kinds. Each accepted
-- `ext:<unit>:<kind>` since 1794470011, and no code writes one into them:
-- attachments lock a core parent row, embeddings cover core kinds, field
-- provenance and record views are stamped by core stores, duplicates are
-- proposed for contacts, companies and leads, and a signal is about a core
-- record. record_grant stays open until row access for extension records is
-- decided.
--
-- dedupe_candidate loses its extension branch: the two id columns, their shape
-- CHECK and their index. No code reads or writes them.
--
-- The CHECKs are NOT VALID, and the next migration validates them.
SET LOCAL lock_timeout = '3s';

ALTER TABLE attachment DROP CONSTRAINT IF EXISTS attachment_entity_type_check;
ALTER TABLE attachment
    ADD CONSTRAINT attachment_entity_type_check CHECK (entity_type IN ('contact', 'company', 'deal', 'lead', 'activity', 'project', 'relationship', 'partner', 'product', 'offer_template', 'offer', 'commission', 'deal_room', 'deal_room_thread', 'deal_room_comment', 'deal_room_document', 'deal_room_participant')) NOT VALID;

ALTER TABLE embedding DROP CONSTRAINT IF EXISTS embedding_entity_type_check;
ALTER TABLE embedding
    ADD CONSTRAINT embedding_entity_type_check CHECK (entity_type IN ('contact', 'company', 'deal', 'lead', 'activity', 'project', 'relationship', 'partner', 'product', 'offer_template', 'offer', 'commission', 'deal_room', 'deal_room_thread', 'deal_room_comment', 'deal_room_document', 'deal_room_participant')) NOT VALID;

ALTER TABLE field_provenance DROP CONSTRAINT IF EXISTS field_provenance_object_type_check;
ALTER TABLE field_provenance
    ADD CONSTRAINT field_provenance_object_type_check CHECK (object_type IN ('contact', 'company', 'deal', 'lead', 'activity', 'project', 'relationship', 'partner', 'product', 'offer_template', 'offer', 'commission', 'deal_room', 'deal_room_thread', 'deal_room_comment', 'deal_room_document', 'deal_room_participant')) NOT VALID;

ALTER TABLE user_record_view DROP CONSTRAINT IF EXISTS user_record_view_entity_type_check;
ALTER TABLE user_record_view
    ADD CONSTRAINT user_record_view_entity_type_check CHECK (entity_type IN ('company', 'contact')) NOT VALID;

ALTER TABLE signal DROP CONSTRAINT IF EXISTS signal_entity_type_check;
ALTER TABLE signal
    ADD CONSTRAINT signal_entity_type_check CHECK (
        entity_type IS NULL OR entity_type IN ('deal', 'company', 'contact', 'project')
    ) NOT VALID;

DROP INDEX IF EXISTS uq_dedupe_candidate_pair_ext;
ALTER TABLE dedupe_candidate DROP CONSTRAINT IF EXISTS dedupe_candidate_ext_shape;
ALTER TABLE dedupe_candidate DROP CONSTRAINT IF EXISTS dedupe_candidate_entity_type_check;
ALTER TABLE dedupe_candidate DROP CONSTRAINT IF EXISTS dedupe_candidate_shape;
ALTER TABLE dedupe_candidate DROP COLUMN IF EXISTS left_ext_id, DROP COLUMN IF EXISTS right_ext_id;
ALTER TABLE dedupe_candidate
    ADD CONSTRAINT dedupe_candidate_entity_type_check CHECK (entity_type IN ('contact', 'company', 'lead')) NOT VALID,
    ADD CONSTRAINT dedupe_candidate_shape CHECK (
        (entity_type = 'contact' AND left_contact_id IS NOT NULL AND right_contact_id IS NOT NULL AND left_company_id IS NULL AND right_company_id IS NULL AND left_lead_id IS NULL AND right_lead_id IS NULL)
        OR (entity_type = 'company' AND left_company_id IS NOT NULL AND right_company_id IS NOT NULL AND left_contact_id IS NULL AND right_contact_id IS NULL AND left_lead_id IS NULL AND right_lead_id IS NULL)
        OR (entity_type = 'lead' AND left_lead_id IS NOT NULL AND right_lead_id IS NOT NULL AND left_contact_id IS NULL AND right_contact_id IS NULL AND left_company_id IS NULL AND right_company_id IS NULL)
    ) NOT VALID;
