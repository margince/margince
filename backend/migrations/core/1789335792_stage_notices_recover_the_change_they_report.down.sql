-- Recovered historical facts remain valid on rollback; discarding them would
-- make a notification less truthful. Older readers ignore the extra metadata.
SELECT 1;
