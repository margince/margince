-- Validate the author-pair check on contact: an author is meaningful only beside a
-- source_system. It bound new and changed rows from the day it was added and
-- was never checked against the rows already there, so the schema has been
-- printing a guarantee wider than the one the table made.
--
-- Its own file so the scan takes SHARE UPDATE EXCLUSIVE rather than inheriting
-- the ACCESS EXCLUSIVE the adding migration held until it committed.
SET LOCAL lock_timeout = '3s';

ALTER TABLE contact VALIDATE CONSTRAINT contact_source_author_needs_a_source;
