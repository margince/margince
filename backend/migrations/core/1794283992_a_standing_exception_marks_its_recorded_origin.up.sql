-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '3s';

-- NULL leaves old provenance unknown, since an erased parent also clears the carry link.
ALTER TABLE communication_override ADD COLUMN recorded_directly boolean;
