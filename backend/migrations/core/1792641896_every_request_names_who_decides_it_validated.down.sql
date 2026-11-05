-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- Validation cannot be undone, and nothing needs it to be: the constraints
-- stay as they are, and the earlier migration's down removes them.
SELECT 1;
