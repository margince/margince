-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- The team rules every AI draft follows are written by admin and ops and read
-- by every other seat. Presence-guarded: a hand-set grant stays.
UPDATE role SET permissions = jsonb_set(permissions, '{objects,ai_team_rules}',
  CASE WHEN key IN ('admin','ops') THEN '{"create":false,"read":true,"update":true,"delete":false}'::jsonb
       ELSE '{"create":false,"read":true,"update":false,"delete":false}'::jsonb END)
WHERE is_system AND key IN ('admin','ops','management','manager','rep','read_only') AND permissions ? 'objects' AND NOT (permissions -> 'objects') ? 'ai_team_rules';
