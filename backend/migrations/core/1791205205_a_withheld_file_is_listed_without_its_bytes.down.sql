-- The rows naming withheld files go with the column: before it, a private
-- message carried no attachment rows, and every reader of storage_key assumed
-- an object behind the key. Their names are lost; their bytes never existed.
SET LOCAL lock_timeout = '3s';
DELETE FROM attachment WHERE bytes_withheld;
ALTER TABLE attachment DROP COLUMN IF EXISTS bytes_withheld;
