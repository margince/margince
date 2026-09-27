-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- Who reads every team's coaching week without leading the team.
--
-- The week names each member with a verdict their lead is meant to raise. It
-- used to open to any seat whose row scope reached every row, which let a
-- read-only auditor read a manager's judgement of a named colleague. Reaching
-- every record is not that claim, so it becomes a grant of its own: admin and
-- management hold read, every other seeded role holds nothing, and a lead reads
-- the week of a team they are on without it.
--
-- EVERY ROLE IS NAMED, the zero grants written out rather than left out. A role
-- with no key at all is indistinguishable from one the backfill missed.
--
-- Presence-guarded: an operator who has already hand-set this keeps their
-- setting, because the guard is absence rather than value.
SET LOCAL lock_timeout = '5s';

UPDATE role SET permissions = jsonb_set(
        permissions, '{objects,team_oversight}',
        '{"create":false,"read":false,"update":false,"delete":false}'::jsonb, true)
    WHERE is_system
      AND key IN ('manager', 'ops', 'read_only', 'rep')
      AND permissions ? 'objects'
      AND NOT (permissions -> 'objects') ? 'team_oversight';

UPDATE role SET permissions = jsonb_set(
        permissions, '{objects,team_oversight}',
        '{"create":false,"read":true,"update":false,"delete":false}'::jsonb, true)
    WHERE is_system
      AND key IN ('admin', 'management')
      AND permissions ? 'objects'
      AND NOT (permissions -> 'objects') ? 'team_oversight';
