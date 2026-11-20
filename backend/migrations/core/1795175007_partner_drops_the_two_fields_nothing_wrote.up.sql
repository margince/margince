-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- partner.last_contact_at and partner.relationship_health are dropped. Nothing
-- ever wrote them, and the contract no longer serves them. How cold an account
-- has gone is the dormancy ladder over company.last_activity_at, computed on
-- read. Dropping relationship_health drops its range check with it.
SET LOCAL lock_timeout = '3s';

ALTER TABLE partner DROP COLUMN IF EXISTS last_contact_at,
    DROP COLUMN IF EXISTS relationship_health;
