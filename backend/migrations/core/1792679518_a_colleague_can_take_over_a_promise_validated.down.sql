-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- Validation cannot be undone, and nothing needs it to be: the earlier
-- migration's down drops the constraint.
SELECT 1;
