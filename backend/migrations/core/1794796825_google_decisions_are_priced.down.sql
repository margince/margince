SET LOCAL lock_timeout = '3s';
-- The row the up added stays: it is a price, and removing it would make the
-- model unpriced again on the way up.
SELECT 1;
