SET LOCAL lock_timeout = '5s';

-- Back to the all-false document the earlier migration wrote, only where the
-- grant is still exactly what the up migration set.
UPDATE role SET permissions = jsonb_set(
        permissions, '{objects,communication_exception}',
        '{"create":false,"read":false,"update":false,"delete":false}'::jsonb, true)
    WHERE is_system
      AND key IN ('rep', 'manager', 'management')
      AND permissions ? 'objects'
      AND (permissions -> 'objects' -> 'communication_exception')
          = '{"create":true,"read":true,"update":false,"delete":false}'::jsonb;
