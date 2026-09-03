-- Remove the object from the seeded roles. An operator who hand-set it after
-- the fact loses that setting on a revert, which is the same trade every
-- permission backfill in this tree makes.
SET LOCAL lock_timeout = '5s';

UPDATE role SET permissions = permissions #- '{objects,data_coverage}'
    WHERE is_system AND permissions ? 'objects';
