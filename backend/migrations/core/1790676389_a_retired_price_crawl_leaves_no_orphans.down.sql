-- Nothing to restore. The up migration expired proposals nobody can apply and
-- deleted queue rows for work that had not run; a rollback brings back neither
-- the crawl that staged them nor a row this file could forge with the old id.
SELECT 1;
