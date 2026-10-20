-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- Back to one company per installation. The entities and every assignment go
-- with their tables. The role grants stay: an object the code does not know
-- grants nothing, and Parse drops it.
SET LOCAL lock_timeout = '3s';

DROP TABLE IF EXISTS legal_entity_member;
DROP TABLE IF EXISTS legal_entity;
DROP FUNCTION IF EXISTS legal_entity_refuse_default_retirement();
