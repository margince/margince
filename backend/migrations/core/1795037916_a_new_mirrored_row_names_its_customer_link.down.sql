-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '3s';

ALTER TABLE finance_payment DROP CONSTRAINT IF EXISTS finance_payment_names_its_customer_link;
ALTER TABLE finance_invoice DROP CONSTRAINT IF EXISTS finance_invoice_names_its_customer_link;
