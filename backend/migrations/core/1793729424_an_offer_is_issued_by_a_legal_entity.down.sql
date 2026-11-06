-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- Every offer is issued by the installation again.
SET LOCAL lock_timeout = '3s';

DROP INDEX IF EXISTS idx_offer_legal_entity;
ALTER TABLE offer DROP COLUMN IF EXISTS legal_entity_id;
