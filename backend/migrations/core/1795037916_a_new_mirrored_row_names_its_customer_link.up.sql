-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- Every invoice and payment the sync writes or rewrites names the customer link
-- it came through. The checks are NOT VALID and stay so: a row mirrored before
-- the column existed may name no link, and it stays as it is until a sync or a
-- person names one. permanentlyUnvalidated in the gates says why.
--
-- Each check is dropped if present and added again, so a second run leaves the
-- same schema. A DO block that adds one would read to the gates as a data rewrite.
SET LOCAL lock_timeout = '3s';

ALTER TABLE finance_invoice
    DROP CONSTRAINT IF EXISTS finance_invoice_names_its_customer_link,
    ADD CONSTRAINT finance_invoice_names_its_customer_link CHECK (customer_link_id IS NOT NULL) NOT VALID;

ALTER TABLE finance_payment
    DROP CONSTRAINT IF EXISTS finance_payment_names_its_customer_link,
    ADD CONSTRAINT finance_payment_names_its_customer_link CHECK (customer_link_id IS NOT NULL) NOT VALID;
