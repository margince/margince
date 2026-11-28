-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '3s';

ALTER TABLE contact DROP CONSTRAINT contact_salutation_check;
ALTER TABLE contact ADD CONSTRAINT contact_salutation_check CHECK (salutation IN ('mr', 'ms', 'none')) NOT VALID;
ALTER TABLE lead DROP CONSTRAINT lead_salutation_check;
ALTER TABLE lead ADD CONSTRAINT lead_salutation_check CHECK (salutation IN ('mr', 'ms', 'none')) NOT VALID;
