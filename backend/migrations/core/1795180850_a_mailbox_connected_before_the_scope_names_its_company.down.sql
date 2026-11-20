-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- The rows this migration scoped cannot be told from rows scoped on connect,
-- and a scoped connection is correct either way, so there is nothing to undo.
SELECT 1;
