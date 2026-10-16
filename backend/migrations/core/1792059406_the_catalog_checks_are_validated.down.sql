-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '3s';

ALTER TABLE attachment RENAME CONSTRAINT attachment_entity_type_check TO attachment_entity_type_check_v2;
ALTER TABLE attachment ADD CONSTRAINT attachment_entity_type_check CHECK (entity_type IN ('contact', 'company', 'deal', 'lead', 'activity', 'project', 'relationship', 'partner')) NOT VALID;
ALTER TABLE embedding RENAME CONSTRAINT embedding_entity_type_check TO embedding_entity_type_check_v2;
ALTER TABLE embedding ADD CONSTRAINT embedding_entity_type_check CHECK (entity_type IN ('contact', 'company', 'deal', 'lead', 'activity', 'project', 'relationship', 'partner')) NOT VALID;
ALTER TABLE field_provenance RENAME CONSTRAINT field_provenance_object_type_check TO field_provenance_object_type_check_v2;
ALTER TABLE field_provenance ADD CONSTRAINT field_provenance_object_type_check CHECK (object_type IN ('contact', 'company', 'deal', 'lead', 'activity', 'project', 'relationship', 'partner')) NOT VALID;
