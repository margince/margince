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
--
-- Widening a CHECK is additive: every row already stored satisfies the new
-- predicate, because the old values all remain. No backfill, no rewrite.
--
-- It is still done in four steps per table rather than a drop-and-recreate,
-- and the reason is a trap worth naming. A plain ADD CONSTRAINT validates
-- every existing row WHILE HOLDING ACCESS EXCLUSIVE, and `lock_timeout` does
-- not help: it bounds how long the statement waits to ACQUIRE the lock, never
-- how long it holds one once it has it. On a mature embedding or
-- field_provenance table that scan is the outage. NOT VALID skips the scan and
-- takes the lock only long enough to record the constraint; VALIDATE then
-- checks the existing rows under a SHARE UPDATE EXCLUSIVE lock that readers
-- and writers pass.
--
-- The validation cannot fail here, because the new predicate admits everything
-- the old one did. It is run anyway rather than left NOT VALID: an unvalidated
-- constraint is not trusted by the planner and reads as provisional to the
-- next person who looks at the schema.
--
-- WHAT THIS DOES NOT BUY, stated so nobody reads more safety into it than is
-- here. dbmigrate runs each migration file inside ONE transaction, so the
-- ACCESS EXCLUSIVE taken by ADD CONSTRAINT is held until this file commits —
-- the VALIDATE scan runs under it rather than under the SHARE UPDATE EXCLUSIVE
-- it would take on its own connection. Splitting the two across separately
-- committed migrations is the only way to get that, and that is a change to
-- how every migration in this tree runs, not a choice this one may make.
--
-- What the staging still buys is the shape: the swap is additive and
-- reversible, the constraint keeps its name, and the day the migrator learns
-- to commit phases this file needs no rewrite. The four tables here are also
-- the small end — embedding is the largest and the scan is a sequential read
-- of one text column — so the exposure is bounded today even though the
-- mechanism is not.
SET LOCAL lock_timeout = '3s';

ALTER TABLE attachment ADD CONSTRAINT attachment_entity_type_check_v2
    CHECK (entity_type IN ('person', 'organization', 'deal', 'lead', 'activity', 'project', 'relationship', 'partner')) NOT VALID;
ALTER TABLE attachment VALIDATE CONSTRAINT attachment_entity_type_check_v2;
ALTER TABLE attachment DROP CONSTRAINT attachment_entity_type_check;
ALTER TABLE attachment RENAME CONSTRAINT attachment_entity_type_check_v2 TO attachment_entity_type_check;

ALTER TABLE embedding ADD CONSTRAINT embedding_entity_type_check_v2
    CHECK (entity_type IN ('person', 'organization', 'deal', 'lead', 'activity', 'project', 'relationship', 'partner')) NOT VALID;
ALTER TABLE embedding VALIDATE CONSTRAINT embedding_entity_type_check_v2;
ALTER TABLE embedding DROP CONSTRAINT embedding_entity_type_check;
ALTER TABLE embedding RENAME CONSTRAINT embedding_entity_type_check_v2 TO embedding_entity_type_check;

ALTER TABLE field_provenance ADD CONSTRAINT field_provenance_object_type_check_v2
    CHECK (object_type IN ('person', 'organization', 'deal', 'lead', 'activity', 'project', 'relationship', 'partner')) NOT VALID;
ALTER TABLE field_provenance VALIDATE CONSTRAINT field_provenance_object_type_check_v2;
ALTER TABLE field_provenance DROP CONSTRAINT field_provenance_object_type_check;
ALTER TABLE field_provenance RENAME CONSTRAINT field_provenance_object_type_check_v2 TO field_provenance_object_type_check;

ALTER TABLE custom_field ADD CONSTRAINT custom_field_object_check_v2
    CHECK (object IN ('person', 'organization', 'deal', 'lead', 'activity', 'project', 'relationship', 'partner')) NOT VALID;
ALTER TABLE custom_field VALIDATE CONSTRAINT custom_field_object_check_v2;
ALTER TABLE custom_field DROP CONSTRAINT custom_field_object_check;
ALTER TABLE custom_field RENAME CONSTRAINT custom_field_object_check_v2 TO custom_field_object_check;
