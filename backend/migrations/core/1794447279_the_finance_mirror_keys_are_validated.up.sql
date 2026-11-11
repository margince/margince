-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- The keys the previous migration added are validated here, under SHARE
-- UPDATE EXCLUSIVE, which readers and writers pass.
--
-- A link made before its directory entry existed, as the demo seed once did,
-- gets the entry it names, named by its own id. The next sync replaces the
-- name with the source's and archives nothing. Every other key holds on
-- existing rows: a link's rows were written in one transaction with the link's
-- company, and a row mirrored before customer_link_id existed holds NULL, which
-- the key leaves unchecked until the sync names its link.
SET LOCAL lock_timeout = '3s';

INSERT INTO finance_external_customer (connection_id, external_customer_id, display_name, sync_hash, source, captured_by)
SELECT DISTINCT l.connection_id, l.external_customer_id, l.external_customer_id, '', 'system', 'system:migration'
  FROM finance_customer_link l
 WHERE NOT EXISTS (SELECT 1 FROM finance_external_customer c
                    WHERE c.connection_id = l.connection_id AND c.external_customer_id = l.external_customer_id);

ALTER TABLE finance_customer_link VALIDATE CONSTRAINT finance_customer_link_directory_fk;
ALTER TABLE finance_invoice VALIDATE CONSTRAINT finance_invoice_customer_link_fk;
ALTER TABLE finance_invoice VALIDATE CONSTRAINT finance_invoice_credits_owner_fk;
ALTER TABLE finance_invoice VALIDATE CONSTRAINT finance_invoice_credit_note_status;
ALTER TABLE finance_payment VALIDATE CONSTRAINT finance_payment_customer_link_fk;
ALTER TABLE finance_payment VALIDATE CONSTRAINT finance_payment_invoice_owner_fk;
