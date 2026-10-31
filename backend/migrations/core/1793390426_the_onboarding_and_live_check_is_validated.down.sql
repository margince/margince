-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- A constraint cannot be un-validated. Restoring the old list is the previous
-- file's own down.
SET LOCAL lock_timeout = '3s';
