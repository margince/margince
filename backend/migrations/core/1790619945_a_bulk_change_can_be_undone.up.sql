-- A bulk change can be undone once, by a compensating bulk change of its own.
--
-- result keeps what the change did per record: the version it left each
-- changed record at and the owner it took from it, which is what the undo
-- compares and puts back, and every record it left alone with the reason.
-- undo_of links an undo to the change it reversed; undone_by is the same link
-- read from the other end, written in the undo's transaction.
SET LOCAL lock_timeout = '5s';

ALTER TABLE bulk_operation
    ADD COLUMN result jsonb NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN undo_of uuid,
    ADD COLUMN undone_by uuid;

-- One undo per change, whichever of two racing undos commits first.
CREATE UNIQUE INDEX uq_bulk_operation_undo_of ON bulk_operation (undo_of) WHERE undo_of IS NOT NULL;
