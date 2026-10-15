-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- Validates the wider CHECKs 1792069478 recorded, in a transaction of its own so the
-- scan runs under SHARE UPDATE EXCLUSIVE, then retires the narrower ones under
-- the names they had. Every stored row passes: the new set admits all the old.
SET LOCAL lock_timeout = '3s';

ALTER TABLE attachment VALIDATE CONSTRAINT attachment_entity_type_check_v2;
ALTER TABLE embedding VALIDATE CONSTRAINT embedding_entity_type_check_v2;
ALTER TABLE field_provenance VALIDATE CONSTRAINT field_provenance_object_type_check_v2;

ALTER TABLE attachment DROP CONSTRAINT attachment_entity_type_check;
ALTER TABLE attachment RENAME CONSTRAINT attachment_entity_type_check_v2 TO attachment_entity_type_check;
ALTER TABLE embedding DROP CONSTRAINT embedding_entity_type_check;
ALTER TABLE embedding RENAME CONSTRAINT embedding_entity_type_check_v2 TO embedding_entity_type_check;
ALTER TABLE field_provenance DROP CONSTRAINT field_provenance_object_type_check;
ALTER TABLE field_provenance RENAME CONSTRAINT field_provenance_object_type_check_v2 TO field_provenance_object_type_check;
