-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- The keys the previous migration added are validated here, under SHARE
-- UPDATE EXCLUSIVE, which readers and writers pass.
--
-- A link made before its directory entry existed, as the demo seed once did,
-- gets the entry it names, named by its own id, with the create audit row
-- mirrorCustomer writes. The next sync replaces the name with the source's.
--
-- Every other key holds on existing rows: a link's rows were written in one
-- transaction with the link's company. A row mirrored before customer_link_id
-- existed holds NULL, which the key leaves unchecked. The data task
-- core/1794447279_customer_link_backfill names the link on those rows, and is
-- the precondition for a NOT NULL check on the column in a later release.
SET LOCAL lock_timeout = '3s';

WITH stub AS (
    INSERT INTO finance_external_customer (connection_id, external_customer_id, display_name, sync_hash, source, captured_by)
    SELECT DISTINCT l.connection_id, l.external_customer_id, l.external_customer_id, '', 'system', 'system:migration'
      FROM finance_customer_link l
     WHERE NOT EXISTS (SELECT 1 FROM finance_external_customer c
                        WHERE c.connection_id = l.connection_id AND c.external_customer_id = l.external_customer_id)
    RETURNING id, display_name, sync_hash
)
INSERT INTO audit_log (actor_type, actor_id, action, entity_type, entity_id, after)
SELECT 'system', 'system:migration', 'create', 'finance_external_customer', id,
       jsonb_build_object('display_name', display_name, 'sync_hash', sync_hash)
  FROM stub;

ALTER TABLE finance_customer_link VALIDATE CONSTRAINT finance_customer_link_directory_fk;
ALTER TABLE finance_invoice VALIDATE CONSTRAINT finance_invoice_customer_link_fk;
ALTER TABLE finance_invoice VALIDATE CONSTRAINT finance_invoice_credits_owner_fk;
ALTER TABLE finance_invoice VALIDATE CONSTRAINT finance_invoice_credit_note_status;
ALTER TABLE finance_payment VALIDATE CONSTRAINT finance_payment_customer_link_fk;
ALTER TABLE finance_payment VALIDATE CONSTRAINT finance_payment_invoice_owner_fk;
