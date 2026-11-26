-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- The finance mirror's rows stop naming their customer link.
SET LOCAL lock_timeout = '3s';

ALTER TABLE finance_payment
    DROP CONSTRAINT finance_payment_invoice_owner_fk,
    DROP CONSTRAINT finance_payment_customer_link_fk,
    DROP COLUMN customer_link_id;

ALTER TABLE finance_invoice
    DROP CONSTRAINT finance_invoice_credit_note_status,
    DROP CONSTRAINT finance_invoice_credits_owner_fk,
    DROP CONSTRAINT finance_invoice_customer_link_fk,
    DROP CONSTRAINT finance_invoice_owner_key,
    DROP COLUMN customer_link_id;

ALTER TABLE finance_customer_link
    DROP CONSTRAINT finance_customer_link_directory_fk,
    DROP CONSTRAINT finance_customer_link_mapping_key;
