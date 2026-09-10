SET LOCAL lock_timeout = '5s';

-- The object is removed from every system role. A hand-set grant on a CUSTOM
-- role is left alone: it was somebody's decision, and this migration never
-- made it.
UPDATE role SET permissions = permissions #- '{objects,communication_exception}'
    WHERE is_system AND permissions ? 'objects';
