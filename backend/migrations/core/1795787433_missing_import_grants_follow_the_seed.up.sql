SET LOCAL lock_timeout = '3s';

-- Fresh installations have this object in policy. An older role may have no
-- object at all, so an all-false-only predicate cannot bring it up to date.
-- Existing grants, including an explicit denial, remain the admin's choice.
UPDATE role AS existing
SET permissions = jsonb_set(existing.permissions, '{objects,import_run}', seeded.permissions_value)
FROM (VALUES
    ('admin', '{"create":true,"delete":true,"read":true,"update":true}'::jsonb),
    ('management', '{"create":true,"delete":false,"read":true,"update":true}'::jsonb),
    ('manager', '{"create":true,"delete":false,"read":true,"update":true}'::jsonb),
    ('ops', '{"create":true,"delete":true,"read":true,"update":true}'::jsonb),
    ('read_only', '{"create":false,"delete":false,"read":false,"update":false}'::jsonb),
    ('rep', '{"create":true,"delete":false,"read":true,"update":true}'::jsonb)
) AS seeded(role_key, permissions_value)
WHERE existing.is_system AND existing.key = seeded.role_key
  AND existing.permissions ? 'objects'
  AND NOT (existing.permissions->'objects' ? 'import_run');
