SET LOCAL lock_timeout = '5s';

-- Guarded on the value the up wrote, for the reason it guards on the one it
-- replaced: an operator who has edited forecast since keeps their edit.
UPDATE role SET permissions = jsonb_set(
        permissions, '{objects,forecast,update}', 'false'::jsonb, false)
    WHERE is_system
      AND key IN ('admin', 'management', 'manager', 'ops')
      AND permissions -> 'objects' -> 'forecast'
          = '{"create":true,"read":true,"update":true,"delete":false}'::jsonb;

UPDATE role SET permissions = jsonb_set(
        permissions, '{objects,forecast,update}', 'false'::jsonb, false)
    WHERE is_system
      AND key = 'rep'
      AND permissions -> 'objects' -> 'forecast'
          = '{"create":false,"read":true,"update":true,"delete":false}'::jsonb;
