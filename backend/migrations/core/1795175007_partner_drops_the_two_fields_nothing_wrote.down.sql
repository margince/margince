-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- The two columns come back empty, as they always were.
SET LOCAL lock_timeout = '3s';

ALTER TABLE partner ADD COLUMN IF NOT EXISTS relationship_health numeric(3,2),
    ADD COLUMN IF NOT EXISTS last_contact_at timestamptz,
    ADD CONSTRAINT partner_relationship_health_check
        CHECK ((relationship_health IS NULL) OR ((relationship_health >= (0)::numeric) AND (relationship_health <= (1)::numeric))) NOT VALID;
