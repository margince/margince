-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- Postgres has no ALTER that un-validates, and the file before this one drops
-- every check it added, so nothing is left to put back.
SELECT 1;
