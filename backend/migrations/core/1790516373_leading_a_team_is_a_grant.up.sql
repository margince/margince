-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- Who leads the teams they are on.
--
-- Coaching a teammate and opening a led team's week used to be decided by the
-- role KEY: admin, management and manager led their teams, every other key did
-- not. A key list admits only the seats it names, so a role an operator made
-- could never lead a team. The authority becomes the team_lead grant: create
-- raises a coaching notice, read opens the week.
--
-- THE BACKFILL NEVER WIDENS. Every admin, management and manager role led its
-- teams under the key rule whatever its document said, so granting them
-- create+read keeps exactly what they had. Every other system role led nobody
-- and gets the zero grant. A role an operator made (is_system false) led nobody
-- either, and is left without the key, which grants nothing.
--
-- EVERY SYSTEM ROLE IS NAMED, the zero grants written out rather than left out.
-- A role with no key at all is indistinguishable from one the backfill missed.
--
-- Presence-guarded: an operator who has already hand-set this keeps their
-- setting, because the guard is absence rather than value.
SET LOCAL lock_timeout = '5s';

UPDATE role SET permissions = jsonb_set(
        permissions, '{objects,team_lead}',
        '{"create":true,"read":true,"update":false,"delete":false}'::jsonb, true)
    WHERE is_system
      AND key IN ('admin', 'management', 'manager')
      AND permissions ? 'objects'
      AND NOT (permissions -> 'objects') ? 'team_lead';

UPDATE role SET permissions = jsonb_set(
        permissions, '{objects,team_lead}',
        '{"create":false,"read":false,"update":false,"delete":false}'::jsonb, true)
    WHERE is_system
      AND permissions ? 'objects'
      AND NOT (permissions -> 'objects') ? 'team_lead';
