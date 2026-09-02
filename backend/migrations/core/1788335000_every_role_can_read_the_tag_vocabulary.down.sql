-- Remove the key this migration added.
--
-- It cannot tell a document it wrote from one that already carried tag, so it
-- removes the key from the system roles wholesale — which is the state an
-- installation predating 1788320100 was in, and the state the up migration is
-- a no-op against on re-run.
SET LOCAL lock_timeout = '3s';

UPDATE role SET permissions = permissions #- '{objects,tag}'
    WHERE is_system
      AND key IN ('admin', 'ops', 'management', 'manager', 'rep', 'read_only')
      AND permissions ? 'objects'
      AND (permissions -> 'objects') ? 'tag';
