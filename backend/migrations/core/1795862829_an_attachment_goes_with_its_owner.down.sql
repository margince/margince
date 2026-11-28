-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '3s';
DROP TRIGGER trg_attachment_owner_keys ON attachment;
DROP FUNCTION trg_attachment_owner_keys();
DROP TRIGGER trg_attachment_updated ON attachment;
CREATE TRIGGER trg_attachment_updated BEFORE UPDATE ON attachment FOR EACH ROW EXECUTE FUNCTION set_updated_at();
ALTER TABLE attachment DROP CONSTRAINT IF EXISTS attachment_entity_type_check;
ALTER TABLE attachment ADD CONSTRAINT attachment_entity_type_check CHECK (entity_type IN ('contact', 'company', 'deal', 'lead', 'activity', 'project', 'relationship', 'partner', 'product', 'offer_template', 'offer', 'commission', 'deal_room', 'deal_room_thread', 'deal_room_comment', 'deal_room_document', 'deal_room_participant')) NOT VALID;
ALTER TABLE attachment
    DROP COLUMN owner_unkeyed,
    DROP COLUMN owner_contact_id,
    DROP COLUMN owner_company_id,
    DROP COLUMN owner_deal_id,
    DROP COLUMN owner_lead_id,
    DROP COLUMN owner_project_id,
    DROP COLUMN owner_activity_id;
