SET LOCAL lock_timeout = '3s';
-- A validated foreign key stays valid; the up's own down removes it.
SELECT 1;
