-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- Put the check back the way the file beside this one found it: present,
-- unvalidated. Postgres has no ALTER that un-validates, so it is dropped and
-- re-added NOT VALID, which takes ACCESS EXCLUSIVE without a scan.
SET LOCAL lock_timeout = '3s';

ALTER TABLE offer
    DROP CONSTRAINT offer_language_shape,
    ADD CONSTRAINT offer_language_shape CHECK (language IS NULL OR language ~ '^[a-z]{2}$') NOT VALID;
