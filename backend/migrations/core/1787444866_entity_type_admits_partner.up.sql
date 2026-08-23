-- The partner extension on an organization becomes an entity the record seam
-- serves, so the four EntityType-bound CHECKs widen to carry it.
-- TestEveryDomainEnumMatchesItsSchemaCheck derives the Go set from the
-- datasource package's constants and compares it against these four, so
-- declaring EntityPartner without this migration fails the gate — which is
-- what makes a half-widening impossible rather than merely discouraged.
--
-- What each widening opens, and what it does not:
--   attachment       — a file may hang off a partner record.
--   embedding        — partner text may be indexed for retrieval.
--   field_provenance — a partner field edit records who made it.
--   custom_field     — NOT opened for authoring: customfields.FieldObjects is
--                      the acceptance set and partner is absent from it, so
--                      the CHECK admits the value while the engine refuses to
--                      create one. The catalog CHECK staying wider than that
--                      list is the posture the other three already have.
SET LOCAL lock_timeout = '5s';

ALTER TABLE attachment DROP CONSTRAINT attachment_entity_type_check;
ALTER TABLE attachment ADD CONSTRAINT attachment_entity_type_check
    CHECK (entity_type IN ('person', 'organization', 'deal', 'lead', 'activity', 'project', 'relationship', 'partner'));

ALTER TABLE embedding DROP CONSTRAINT embedding_entity_type_check;
ALTER TABLE embedding ADD CONSTRAINT embedding_entity_type_check
    CHECK (entity_type IN ('person', 'organization', 'deal', 'lead', 'activity', 'project', 'relationship', 'partner'));

ALTER TABLE field_provenance DROP CONSTRAINT field_provenance_object_type_check;
ALTER TABLE field_provenance ADD CONSTRAINT field_provenance_object_type_check
    CHECK (object_type IN ('person', 'organization', 'deal', 'lead', 'activity', 'project', 'relationship', 'partner'));

ALTER TABLE custom_field DROP CONSTRAINT custom_field_object_check;
ALTER TABLE custom_field ADD CONSTRAINT custom_field_object_check
    CHECK (object IN ('person', 'organization', 'deal', 'lead', 'activity', 'project', 'relationship', 'partner'));
