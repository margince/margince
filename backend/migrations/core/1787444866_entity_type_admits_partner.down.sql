-- The inverse widening, narrowed back. Rows naming the partner entity would
-- violate the tighter predicate, so they are removed first — and unlike the up
-- direction this genuinely can fail, which is why each delete says what it
-- would be destroying.
--
-- attachment is the one that can REFUSE rather than delete: a deal room's
-- document, thread and decision references are ON DELETE RESTRICT, so an
-- attachment on a partner that somebody put in a deal room stops this
-- migration with a foreign-key violation. That is the correct outcome — the
-- alternative is cascading through negotiation evidence to make a rollback
-- tidy. If it fires, remove the deal-room references first and decide
-- deliberately what happens to that evidence.
--
-- custom_field never accepts 'partner' through the engine
-- (customfields.FieldObjects excludes it), so its delete is a sweep for
-- completeness rather than an expected loss.
--
-- The staged swap is the same shape as the up direction and for the same
-- reason: a plain ADD CONSTRAINT would scan the whole table under ACCESS
-- EXCLUSIVE. Here the validation CAN fail if a partner row survived the
-- deletes above, which is the honest failure mode for a narrowing.
SET LOCAL lock_timeout = '3s';

DELETE FROM embedding WHERE entity_type = 'partner';
DELETE FROM field_provenance WHERE object_type = 'partner';
DELETE FROM custom_field WHERE object = 'partner';
DELETE FROM attachment WHERE entity_type = 'partner';

ALTER TABLE attachment ADD CONSTRAINT attachment_entity_type_check_v2
    CHECK (entity_type IN ('person', 'organization', 'deal', 'lead', 'activity', 'project', 'relationship')) NOT VALID;
ALTER TABLE attachment VALIDATE CONSTRAINT attachment_entity_type_check_v2;
ALTER TABLE attachment DROP CONSTRAINT attachment_entity_type_check;
ALTER TABLE attachment RENAME CONSTRAINT attachment_entity_type_check_v2 TO attachment_entity_type_check;

ALTER TABLE embedding ADD CONSTRAINT embedding_entity_type_check_v2
    CHECK (entity_type IN ('person', 'organization', 'deal', 'lead', 'activity', 'project', 'relationship')) NOT VALID;
ALTER TABLE embedding VALIDATE CONSTRAINT embedding_entity_type_check_v2;
ALTER TABLE embedding DROP CONSTRAINT embedding_entity_type_check;
ALTER TABLE embedding RENAME CONSTRAINT embedding_entity_type_check_v2 TO embedding_entity_type_check;

ALTER TABLE field_provenance ADD CONSTRAINT field_provenance_object_type_check_v2
    CHECK (object_type IN ('person', 'organization', 'deal', 'lead', 'activity', 'project', 'relationship')) NOT VALID;
ALTER TABLE field_provenance VALIDATE CONSTRAINT field_provenance_object_type_check_v2;
ALTER TABLE field_provenance DROP CONSTRAINT field_provenance_object_type_check;
ALTER TABLE field_provenance RENAME CONSTRAINT field_provenance_object_type_check_v2 TO field_provenance_object_type_check;

ALTER TABLE custom_field ADD CONSTRAINT custom_field_object_check_v2
    CHECK (object IN ('person', 'organization', 'deal', 'lead', 'activity', 'project', 'relationship')) NOT VALID;
ALTER TABLE custom_field VALIDATE CONSTRAINT custom_field_object_check_v2;
ALTER TABLE custom_field DROP CONSTRAINT custom_field_object_check;
ALTER TABLE custom_field RENAME CONSTRAINT custom_field_object_check_v2 TO custom_field_object_check;
