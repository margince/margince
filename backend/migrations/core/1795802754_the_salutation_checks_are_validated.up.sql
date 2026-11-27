-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '3s';

ALTER TABLE contact VALIDATE CONSTRAINT contact_salutation_check;
ALTER TABLE lead VALIDATE CONSTRAINT lead_salutation_check;
