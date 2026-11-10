-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- A validated CHECK cannot be made unvalidated; the previous migration's down
-- replaces them, so there is nothing to undo here.
SELECT 1;
