-- Reach the installations that already exist. seedSystemRoles writes each role
-- document once at workspace creation and never re-syncs, so an object added to
-- the compiled defaults alone is granted to nobody who bootstrapped earlier — it
-- works on a fresh database and 403s everywhere else, permanently. The grants
-- below are the same ones the compiled defaults carry.
SET LOCAL lock_timeout = '5s';

-- Defining a corpus, editing its words and moving its grounding floor are
-- workspace config: the floor decides what the product will refuse to answer for
-- everyone. Uploading a document puts third-party prose into the body every seat
-- then asks, and the delete is a hard one that takes the chunks, the vectors and
-- the stored file with it.
UPDATE role SET permissions = jsonb_set(
        permissions, '{objects,knowledge_corpus}',
        '{"create": true, "read": true, "update": true, "delete": true}'::jsonb, true)
    WHERE key IN ('admin', 'ops')
      AND permissions ? 'objects'
      AND NOT (permissions -> 'objects') ? 'knowledge_corpus';

UPDATE role SET permissions = jsonb_set(
        permissions, '{objects,knowledge_document}',
        '{"create": true, "read": true, "update": true, "delete": true}'::jsonb, true)
    WHERE key IN ('admin', 'ops')
      AND permissions ? 'objects'
      AND NOT (permissions -> 'objects') ? 'knowledge_document';

-- Read is the ask. Asking a corpus returns its content, and anything that
-- returns a record is a read — so every role that reads records holds this, or
-- the help bot is an admin tool. The document read is what lets the person who
-- received a cited answer open what it cited; an answer whose source is
-- unreadable is not a citation.
UPDATE role SET permissions = jsonb_set(
        permissions, '{objects,knowledge_corpus}',
        '{"create": false, "read": true, "update": false, "delete": false}'::jsonb, true)
    WHERE key IN ('manager', 'management', 'rep', 'read_only')
      AND permissions ? 'objects'
      AND NOT (permissions -> 'objects') ? 'knowledge_corpus';

UPDATE role SET permissions = jsonb_set(
        permissions, '{objects,knowledge_document}',
        '{"create": false, "read": true, "update": false, "delete": false}'::jsonb, true)
    WHERE key IN ('manager', 'management', 'rep', 'read_only')
      AND permissions ? 'objects'
      AND NOT (permissions -> 'objects') ? 'knowledge_document';
