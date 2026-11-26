-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '3s';

ALTER TABLE graph_contact_edge DROP COLUMN IF EXISTS last_precision;
ALTER TABLE graph_interaction_edge DROP COLUMN IF EXISTS last_precision;
