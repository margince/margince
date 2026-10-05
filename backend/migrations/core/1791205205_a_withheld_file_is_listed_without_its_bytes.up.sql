-- A file a private message carried is recorded by name, size and type with no
-- stored object behind it, so its owner still sees what was attached. The flag
-- is what every reader of the bytes asks before it reaches for a storage key,
-- which such a row leaves empty.
--
-- A constant default is stored in the catalog, not written to each row, so the
-- ADD rewrites nothing on a table of every attachment ever captured.
SET LOCAL lock_timeout = '3s';
ALTER TABLE attachment ADD COLUMN bytes_withheld boolean NOT NULL DEFAULT false;
