-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '3s';

ALTER TABLE activity_link VALIDATE CONSTRAINT activity_link_entity_type_check;
ALTER TABLE activity_link VALIDATE CONSTRAINT activity_link_shape;
ALTER TABLE activity_link VALIDATE CONSTRAINT activity_link_ext_id_matches_type;
ALTER TABLE attachment VALIDATE CONSTRAINT attachment_entity_type_check;
ALTER TABLE embedding VALIDATE CONSTRAINT embedding_entity_type_check;
ALTER TABLE field_provenance VALIDATE CONSTRAINT field_provenance_object_type_check;
ALTER TABLE list VALIDATE CONSTRAINT list_entity_type_check;
ALTER TABLE list_member VALIDATE CONSTRAINT list_member_entity_type_check;
ALTER TABLE list_live_member VALIDATE CONSTRAINT list_live_member_entity_type_check;
ALTER TABLE list_member_event VALIDATE CONSTRAINT list_member_event_entity_type_check;
ALTER TABLE taggable VALIDATE CONSTRAINT taggable_entity_type_check;
ALTER TABLE record_grant VALIDATE CONSTRAINT record_grant_record_type_check;
ALTER TABLE record_grant VALIDATE CONSTRAINT record_grant_record_shape;
ALTER TABLE dedupe_candidate VALIDATE CONSTRAINT dedupe_candidate_entity_type_check;
ALTER TABLE dedupe_candidate VALIDATE CONSTRAINT dedupe_candidate_shape;
ALTER TABLE dedupe_candidate VALIDATE CONSTRAINT dedupe_candidate_ext_shape;
ALTER TABLE user_record_view VALIDATE CONSTRAINT user_record_view_entity_type_check;
ALTER TABLE signal VALIDATE CONSTRAINT signal_entity_type_check;
ALTER TABLE saved_view VALIDATE CONSTRAINT saved_view_resource_check;
