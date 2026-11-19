SET LOCAL lock_timeout = '3s';
-- A validated check stays valid; the up's own down removes it.
SELECT 1;
