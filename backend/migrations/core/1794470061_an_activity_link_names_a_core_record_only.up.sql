-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- An activity link names a core record again. The column for an extension
-- record's id and the CHECKs that widened the kind were added with the record
-- kinds, but no code ever wrote a link to an extension record: a message
-- suggestion cannot target one, and no relink reads the column. A column with
-- no writer is a promise the reference page should not make, so it goes.
--
-- The CHECKs are NOT VALID, and the next migration validates them. The
-- statements that change a table take ACCESS EXCLUSIVE on a table this
-- migration did not create, so the wait for it is bounded.
SET LOCAL lock_timeout = '3s';

DELETE FROM activity_link WHERE entity_type LIKE 'ext:%';

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
