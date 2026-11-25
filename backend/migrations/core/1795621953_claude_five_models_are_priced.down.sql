SET LOCAL lock_timeout = '3s';
-- The rows the up added stay: they are prices, and removing them would make the
-- models unpriced again on the way up.
SELECT 1;
