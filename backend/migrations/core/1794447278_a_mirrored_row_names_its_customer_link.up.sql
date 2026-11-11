-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- Money in the finance mirror cannot sit on a company its connection never
-- mapped. Each key below is NOT VALID here and validated by the next migration.
--
-- A customer link names an entry of the source's own directory. The sync
-- mirrors the directory before it reads any link, and a link is made only for
-- an entry the directory lists.
--
-- An invoice and a payment name the customer link they were mirrored through,
-- and carry that link's connection and company. A link never changes its
-- company: moving a customer archives the link and makes a new one, and the
-- next sync moves the mirrored rows onto it. Unlinking archives the link and
-- leaves its mirrored rows on it, so no unlink deletes payment history.
--
-- A payment and a credit note point only at an invoice of the same connection
-- and company. A credit note is credited, or void when the source voided it.
--
-- The link keys are DEFERRED, because a company merge moves the link, its
-- invoices and its payments in three statements of one transaction.
SET LOCAL lock_timeout = '3s';

ALTER TABLE finance_customer_link
    ADD CONSTRAINT finance_customer_link_mapping_key UNIQUE (id, connection_id, company_id),
    ADD CONSTRAINT finance_customer_link_directory_fk
        FOREIGN KEY (connection_id, external_customer_id)
        REFERENCES finance_external_customer (connection_id, external_customer_id) ON DELETE RESTRICT
        NOT VALID;

ALTER TABLE finance_invoice
    ADD COLUMN customer_link_id uuid,
    ADD CONSTRAINT finance_invoice_owner_key UNIQUE (id, connection_id, company_id),
    ADD CONSTRAINT finance_invoice_customer_link_fk
        FOREIGN KEY (customer_link_id, connection_id, company_id)
        REFERENCES finance_customer_link (id, connection_id, company_id) ON DELETE RESTRICT
        DEFERRABLE INITIALLY DEFERRED NOT VALID,
    ADD CONSTRAINT finance_invoice_credits_owner_fk
        FOREIGN KEY (credits_invoice_id, connection_id, company_id)
        REFERENCES finance_invoice (id, connection_id, company_id) ON DELETE RESTRICT
        DEFERRABLE INITIALLY DEFERRED NOT VALID,
    ADD CONSTRAINT finance_invoice_credit_note_status
        CHECK (credits_invoice_id IS NULL OR status IN ('credited', 'void')) NOT VALID;

ALTER TABLE finance_payment
    ADD COLUMN customer_link_id uuid,
    ADD CONSTRAINT finance_payment_customer_link_fk
        FOREIGN KEY (customer_link_id, connection_id, company_id)
        REFERENCES finance_customer_link (id, connection_id, company_id) ON DELETE RESTRICT
        DEFERRABLE INITIALLY DEFERRED NOT VALID,
    ADD CONSTRAINT finance_payment_invoice_owner_fk
        FOREIGN KEY (invoice_id, connection_id, company_id)
        REFERENCES finance_invoice (id, connection_id, company_id) ON DELETE RESTRICT
        DEFERRABLE INITIALLY DEFERRED NOT VALID;
