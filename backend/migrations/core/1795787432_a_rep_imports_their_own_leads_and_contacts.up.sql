-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- A rep, a manager and management import the leads and contacts they own: they may read, start and
-- commit their own import runs. Discarding any run stays with admin and ops,
-- and is what marks a workspace import. Guarded: a grant somebody already set
-- by hand stays as they set it.
UPDATE role SET permissions = jsonb_set(permissions, '{objects,import_run}',
  '{"create":true,"read":true,"update":true,"delete":false}'::jsonb)
WHERE is_system AND key IN ('rep','manager','management') AND permissions ? 'objects'
  AND coalesce(permissions -> 'objects' -> 'import_run', '{}'::jsonb)
      @> '{"create":false,"read":false,"update":false,"delete":false}'::jsonb;
