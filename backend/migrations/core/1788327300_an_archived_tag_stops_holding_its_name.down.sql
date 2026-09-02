-- Restoring the full-table index can FAIL, and that is the honest behaviour:
-- while the partial index was live the vocabulary may have accepted two
-- archived rows sharing a name, or a live word whose name an archived row also
-- holds. Neither fits a full-table unique index, and there is no correct
-- automatic answer to which row should lose its name.
--
-- A rollback that hits this has to resolve the duplicates by hand first. Silently
-- renaming or deleting one here would destroy a word somebody chose.
SET LOCAL lock_timeout = '3s';

DROP INDEX uq_tag_name;

CREATE UNIQUE INDEX uq_tag_name ON tag USING btree (lower(name));
