-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- A contact has one primary email address and one primary phone number, whatever
-- their types. The slot was one per type, so a contact could hold a primary work
-- and a primary personal address, and the app wrote to whichever the reading
-- order reached first.
--
-- Repair only: where a contact holds several live primaries, keep the one the
-- app already uses (is_primary, position, created_at, then the address or id,
-- the reading order of contactaddress) and demote the rest. Row locks only; the
-- per-contact indexes are built in the next two migrations.
SET LOCAL lock_timeout = '3s';

UPDATE contact_email e SET is_primary = false
 WHERE e.is_primary AND e.archived_at IS NULL
   AND e.id <> (SELECT k.id FROM contact_email k
                 WHERE k.contact_id = e.contact_id AND k.is_primary AND k.archived_at IS NULL
                 ORDER BY k.position, k.created_at, k.email LIMIT 1);

UPDATE contact_phone p SET is_primary = false
 WHERE p.is_primary AND p.archived_at IS NULL
   AND p.id <> (SELECT k.id FROM contact_phone k
                 WHERE k.contact_id = p.contact_id AND k.is_primary AND k.archived_at IS NULL
                 ORDER BY k.position, k.created_at, k.id LIMIT 1);
