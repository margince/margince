-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '2s';

-- Pricing a deal reads the base currency, which is an installation setting, so
-- a service member landing a deal with an amount needs the read every human
-- role already holds. The role is seeded once, on the first declaration, so an
-- installation that already declared a service member holds the old document.
-- The seeded document spells every object, so the guard is that the grant is
-- still the seeded none: a hand-set grant stays.
UPDATE role SET permissions = jsonb_set(
        permissions, '{objects,installation_settings}',
        '{"read": true, "create": false, "update": false, "delete": false}', true)
    WHERE is_system
      AND key = 'service_ingest'
      AND permissions ? 'objects'
      AND permissions #> '{objects,installation_settings}'
          = '{"read": false, "create": false, "update": false, "delete": false}';
