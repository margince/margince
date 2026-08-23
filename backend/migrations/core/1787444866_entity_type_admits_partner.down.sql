-- Rows naming the partner entity would violate the narrower constraints, so
-- they are removed before the CHECKs are put back. custom_field never accepts
-- 'partner' through the engine (customfields.FieldObjects excludes it), so its
-- delete is a sweep for completeness rather than an expected loss.
SET LOCAL lock_timeout = '5s';

DELETE FROM attachment WHERE entity_type = 'partner';
DELETE FROM embedding WHERE entity_type = 'partner';
DELETE FROM field_provenance WHERE object_type = 'partner';
DELETE FROM custom_field WHERE object = 'partner';

ALTER TABLE attachment DROP CONSTRAINT attachment_entity_type_check;
ALTER TABLE attachment ADD CONSTRAINT attachment_entity_type_check
    CHECK (entity_type IN ('person', 'organization', 'deal', 'lead', 'activity', 'project', 'relationship'));

ALTER TABLE embedding DROP CONSTRAINT embedding_entity_type_check;
ALTER TABLE embedding ADD CONSTRAINT embedding_entity_type_check
    CHECK (entity_type IN ('person', 'organization', 'deal', 'lead', 'activity', 'project', 'relationship'));

ALTER TABLE field_provenance DROP CONSTRAINT field_provenance_object_type_check;
ALTER TABLE field_provenance ADD CONSTRAINT field_provenance_object_type_check
    CHECK (object_type IN ('person', 'organization', 'deal', 'lead', 'activity', 'project', 'relationship'));

ALTER TABLE custom_field DROP CONSTRAINT custom_field_object_check;
ALTER TABLE custom_field ADD CONSTRAINT custom_field_object_check
    CHECK (object IN ('person', 'organization', 'deal', 'lead', 'activity', 'project', 'relationship'));
