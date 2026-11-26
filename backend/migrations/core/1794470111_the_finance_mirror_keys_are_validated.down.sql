-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- Nothing to undo here. The previous migration's down drops the keys this one
-- validated, and a directory entry it added is one the next sync keeps current.
SET LOCAL lock_timeout = '3s';
