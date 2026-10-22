-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '3s';

ALTER TABLE attachment DROP CONSTRAINT attachment_entity_type_check_v2;
ALTER TABLE embedding DROP CONSTRAINT embedding_entity_type_check_v2;
ALTER TABLE field_provenance DROP CONSTRAINT field_provenance_object_type_check_v2;
