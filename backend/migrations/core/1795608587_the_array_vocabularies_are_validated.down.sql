-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- Validation is not revertible: a validated check stays validated, and the
-- checks themselves are removed by the file before it.
SELECT 1;
