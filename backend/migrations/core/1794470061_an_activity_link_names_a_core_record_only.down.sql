-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

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
