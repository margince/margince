-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- A member's mailbox or calendar connection made before connections carried a
-- sending company gets the company of its owner: the member's legal entity, or
-- the default one. A connection with no owner (a shared mailbox) stays unscoped.
SET LOCAL lock_timeout = '3s';

UPDATE capture_connection c
   SET legal_entity_id = coalesce(
           (SELECT le.id FROM legal_entity_member m
              JOIN legal_entity le ON le.id = m.legal_entity_id AND le.archived_at IS NULL
             WHERE m.user_id = c.user_id),
           (SELECT id FROM legal_entity WHERE is_default))
 WHERE c.legal_entity_id IS NULL AND c.user_id IS NOT NULL;
