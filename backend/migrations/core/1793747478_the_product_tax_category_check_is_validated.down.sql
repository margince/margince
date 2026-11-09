-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- Put the check back the way the file beside this one found it: present,
-- unvalidated. Postgres has no ALTER that un-validates, so it is dropped and
-- re-added NOT VALID, which takes ACCESS EXCLUSIVE without a scan.
SET LOCAL lock_timeout = '3s';

ALTER TABLE product
    DROP CONSTRAINT product_tax_category_check,
    ADD CONSTRAINT product_tax_category_check
        CHECK (tax_category IS NULL OR tax_category IN ('standard', 'reduced', 'exempt')) NOT VALID;
