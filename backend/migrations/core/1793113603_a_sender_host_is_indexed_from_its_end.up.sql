-- pgmigrate:no-transaction
-- The index a domain question finds its senders' mail by. A sender's host is
-- stored reversed, so a domain and every subdomain of it are one range of the
-- index: "vendor.example" and "mail.vendor.example" both start with
-- "elpmaxe.rodnev". Without it each open question reads every inbound mail in
-- the waiting horizon. Only senders are indexed, since only a sender names the
-- domain a question is about. Built concurrently, so writes are never blocked
-- for the build. The drop ahead of the build clears an invalid index a failed
-- build left behind.
DROP INDEX CONCURRENTLY IF EXISTS idx_aparticipant_sender_host_reversed;
CREATE INDEX CONCURRENTLY idx_aparticipant_sender_host_reversed ON activity_participant
    ((reverse(split_part(lower(address), '@', 2)) COLLATE "C"))
    WHERE role = 'from' AND address IS NOT NULL;
