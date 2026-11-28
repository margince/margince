-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

UPDATE role SET permissions = jsonb_set(permissions, '{objects,import_run}',
  '{"create":false,"read":false,"update":false,"delete":false}'::jsonb)
WHERE is_system AND key IN ('rep','manager','management') AND permissions ? 'objects'
  AND permissions -> 'objects' -> 'import_run'
      = '{"create":true,"read":true,"update":true,"delete":false}'::jsonb;
