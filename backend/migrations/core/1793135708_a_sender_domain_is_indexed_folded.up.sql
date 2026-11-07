-- pgmigrate:no-transaction
-- The index a domain question finds its senders' mail by, on the folded host
-- every participant writer stores. Reversed, a domain and every subdomain of
-- it are one range of the index. The index on the raw host it replaces read a
-- Unicode sender as another domain than its punycode spelling. Built
-- concurrently, so writes are never blocked, and the first drop clears an invalid
-- index a failed build left behind.
DROP INDEX CONCURRENTLY IF EXISTS idx_aparticipant_sender_domain_reversed;
CREATE INDEX CONCURRENTLY idx_aparticipant_sender_domain_reversed ON activity_participant
    ((reverse(address_domain_folded) COLLATE "C"))
    WHERE role = 'from' AND address_domain_folded IS NOT NULL;
DROP INDEX CONCURRENTLY IF EXISTS idx_aparticipant_sender_host_reversed;
