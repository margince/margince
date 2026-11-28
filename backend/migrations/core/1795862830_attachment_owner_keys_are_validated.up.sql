-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '3s';

ALTER TABLE attachment VALIDATE CONSTRAINT attachment_owner_contact_id_fkey;
ALTER TABLE attachment VALIDATE CONSTRAINT attachment_owner_contact_id_bound;
ALTER TABLE attachment VALIDATE CONSTRAINT attachment_owner_company_id_fkey;
ALTER TABLE attachment VALIDATE CONSTRAINT attachment_owner_company_id_bound;
ALTER TABLE attachment VALIDATE CONSTRAINT attachment_owner_deal_id_fkey;
ALTER TABLE attachment VALIDATE CONSTRAINT attachment_owner_deal_id_bound;
ALTER TABLE attachment VALIDATE CONSTRAINT attachment_owner_lead_id_fkey;
ALTER TABLE attachment VALIDATE CONSTRAINT attachment_owner_lead_id_bound;
ALTER TABLE attachment VALIDATE CONSTRAINT attachment_owner_project_id_fkey;
ALTER TABLE attachment VALIDATE CONSTRAINT attachment_owner_project_id_bound;
ALTER TABLE attachment VALIDATE CONSTRAINT attachment_owner_activity_id_fkey;
ALTER TABLE attachment VALIDATE CONSTRAINT attachment_owner_activity_id_bound;
ALTER TABLE attachment VALIDATE CONSTRAINT attachment_owner_shape;
DO $$BEGIN
    ALTER TABLE attachment VALIDATE CONSTRAINT attachment_entity_type_check;
EXCEPTION WHEN check_violation THEN
    -- The guarded shape still refuses every unsupported new owner. Dropping
    -- this vocabulary CHECK lets an old unkeyed row be archived or refiled.
    ALTER TABLE attachment DROP CONSTRAINT attachment_entity_type_check;
    RAISE NOTICE 'attachment keeps older owner kinds for review; the guarded shape refuses new ones';
END $$;
