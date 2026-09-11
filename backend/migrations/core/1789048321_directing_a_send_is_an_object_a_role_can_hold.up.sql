SET LOCAL lock_timeout = '5s';

-- Who may direct a send the engine has refused.
--
-- The engine refuses a message and records why. Sometimes the refusal is right
-- and the message should not go; sometimes the installation has a reason the
-- engine cannot see and a person decides to send anyway. That decision is not
-- a consent grant and must never be recorded as one — it is an exception, taken
-- by a named human, against a refusal that stays on the record.
--
-- ADMIN ALONE by default. Directing a send is the authority to act against the
-- engine's own answer about a data subject, and an installation delegates that
-- by deliberately editing a role rather than by inheriting it here. An owner
-- who wants every rep to hold it grants it; nobody gets it by accident.
--
-- EVERY ROLE IS NAMED, the zero grants written out rather than left out. A role
-- with no key at all is indistinguishable from one the backfill missed, and the
-- next reader auditing this cannot tell a decision from an omission.
--
-- Presence-guarded: an operator who has already hand-set this keeps their
-- setting, because the guard is absence rather than value.
UPDATE role SET permissions = jsonb_set(
        permissions, '{objects,communication_exception}',
        '{"create":false,"read":false,"update":false,"delete":false}'::jsonb, true)
    WHERE is_system
      AND key IN ('management', 'manager', 'ops', 'read_only', 'rep')
      AND permissions ? 'objects'
      AND NOT (permissions -> 'objects') ? 'communication_exception';

UPDATE role SET permissions = jsonb_set(
        permissions, '{objects,communication_exception}',
        '{"create":true,"read":true,"update":true,"delete":true}'::jsonb, true)
    WHERE is_system
      AND key IN ('admin')
      AND permissions ? 'objects'
      AND NOT (permissions -> 'objects') ? 'communication_exception';
