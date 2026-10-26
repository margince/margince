-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '2s';

-- A service member's feed may land deals as well as companies, so the shipped
-- service_ingest role gets the same grant on `deal` it holds on `company`:
-- create, read and update, never delete. The role is seeded once, on the first
-- declaration, so an installation that already declared a service member holds
-- the old document. Presence-guarded: a hand-set grant stays.
UPDATE role SET permissions = jsonb_set(
        permissions, '{objects,deal}', permissions #> '{objects,company}', true)
    WHERE is_system
      AND key = 'service_ingest'
      AND permissions ? 'objects'
      AND (permissions -> 'objects') ? 'company'
      AND NOT (permissions -> 'objects') ? 'deal';
