-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- Who reads every team's coaching week without leading the team.
--
-- The week names each member with a verdict their lead is meant to raise. It
-- used to open to any seat that read deals over every row, which let a
-- read-only auditor read a manager's judgement of a named colleague. Reaching
-- every record is not that claim, so it becomes a grant of its own.
--
-- THE BACKFILL NEVER WIDENS. A role gets read only where its stored document
-- ALREADY opened every team's week under the old rule — row scope all and
-- deal read — and it is admin or management, the two seats that oversee. A
-- management role an operator narrowed to team scope read only its own teams'
-- weeks before, and gets false here rather than every team.
--
-- EVERY ROLE IS NAMED, the zero grants written out rather than left out. A role
-- with no key at all is indistinguishable from one the backfill missed.
--
-- Presence-guarded: an operator who has already hand-set this keeps their
-- setting, because the guard is absence rather than value.
SET LOCAL lock_timeout = '5s';

UPDATE role SET permissions = jsonb_set(
        permissions, '{objects,team_oversight}',
        '{"create":false,"read":true,"update":false,"delete":false}'::jsonb, true)
    WHERE is_system
      AND key IN ('admin', 'management')
      AND permissions ->> 'row_scope' = 'all'
      AND permissions -> 'objects' -> 'deal' ->> 'read' = 'true'
      AND permissions ? 'objects'
      AND NOT (permissions -> 'objects') ? 'team_oversight';

UPDATE role SET permissions = jsonb_set(
        permissions, '{objects,team_oversight}',
        '{"create":false,"read":false,"update":false,"delete":false}'::jsonb, true)
    WHERE is_system
      AND permissions ? 'objects'
      AND NOT (permissions -> 'objects') ? 'team_oversight';
