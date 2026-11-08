-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- Put the check back the way the file beside this one found it: present,
-- unvalidated. Postgres has no ALTER that un-validates, so it is dropped and
-- re-added NOT VALID, which takes ACCESS EXCLUSIVE without a scan.
SET LOCAL lock_timeout = '3s';

ALTER TABLE offer_line_item
    DROP CONSTRAINT offer_line_item_tax_category_check,
    ADD CONSTRAINT offer_line_item_tax_category_check
        CHECK (tax_category IS NULL OR tax_category IN ('standard', 'reduced', 'exempt')) NOT VALID;
