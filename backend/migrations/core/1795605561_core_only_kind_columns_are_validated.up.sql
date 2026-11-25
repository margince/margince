-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- Validates the CHECKs the previous migration added NOT VALID. It is a file of
-- its own so the scan takes only SHARE UPDATE EXCLUSIVE and writers go through.
SET LOCAL lock_timeout = '3s';

ALTER TABLE attachment VALIDATE CONSTRAINT attachment_entity_type_check;
ALTER TABLE embedding VALIDATE CONSTRAINT embedding_entity_type_check;
ALTER TABLE field_provenance VALIDATE CONSTRAINT field_provenance_object_type_check;
ALTER TABLE user_record_view VALIDATE CONSTRAINT user_record_view_entity_type_check;
ALTER TABLE signal VALIDATE CONSTRAINT signal_entity_type_check;
ALTER TABLE dedupe_candidate VALIDATE CONSTRAINT dedupe_candidate_entity_type_check;
ALTER TABLE dedupe_candidate VALIDATE CONSTRAINT dedupe_candidate_shape;
