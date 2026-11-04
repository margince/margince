-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '3s';

DROP INDEX communication_override_legal_entity;
DROP INDEX communication_override_live_contact;
CREATE INDEX communication_override_live_contact
    ON communication_override (contact_id, category) WHERE revoked_at IS NULL;
ALTER TABLE communication_override DROP COLUMN legal_entity_id;
