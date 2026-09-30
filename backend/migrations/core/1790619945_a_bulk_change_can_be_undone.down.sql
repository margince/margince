SET LOCAL lock_timeout = '5s';

DROP INDEX IF EXISTS uq_bulk_operation_undo_of;
ALTER TABLE bulk_operation
    DROP COLUMN IF EXISTS undone_by,
    DROP COLUMN IF EXISTS undo_of,
    DROP COLUMN IF EXISTS result;
