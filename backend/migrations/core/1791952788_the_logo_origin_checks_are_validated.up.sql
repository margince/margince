-- The scan the migration before this one deferred.
--
-- Its own file because VALIDATE takes a lock that a long scan should not hold
-- alongside the ALTER, and because a NOT VALID constraint left unvalidated is a
-- check that refuses new rows while saying nothing about the ones already
-- there.
SET LOCAL lock_timeout = '5s';
ALTER TABLE company VALIDATE CONSTRAINT company_logo_names_its_origin;
ALTER TABLE company VALIDATE CONSTRAINT company_logo_icon_names_its_origin;
