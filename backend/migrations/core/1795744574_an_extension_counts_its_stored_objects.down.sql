-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- Bounded: DROP takes ACCESS EXCLUSIVE and would otherwise queue every write
-- to the table behind it for as long as an open reader holds it.
SET LOCAL lock_timeout = '3s';

DROP TABLE IF EXISTS extension_object_usage;
