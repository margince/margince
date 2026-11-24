-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- A dropped constraint is not restored: it would break the rows that made the
-- upgrade drop it.
SELECT 1;
