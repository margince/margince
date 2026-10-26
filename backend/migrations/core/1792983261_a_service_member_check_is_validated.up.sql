-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- Every service_extension is NULL until an operator declares a member, so the
-- scan passes. VALIDATE takes only SHARE UPDATE EXCLUSIVE, which lets reads and
-- writes go on.
SET LOCAL lock_timeout = '2s';

ALTER TABLE app_user VALIDATE CONSTRAINT app_user_service_is_a_passwordless_agent;
