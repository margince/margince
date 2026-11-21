-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- Validation is not revertible: a validated constraint stays validated, and the
-- check it validates is dropped by the file before this one.
SELECT 1;
