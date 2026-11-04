-- pgmigrate:no-transaction
-- The address-variant lane now also pairs Gmail spellings of one mailbox
-- (contacts addressBaseSQL), so its index takes the new expression and the old
-- one goes. The expression is spelled exactly as the query spells it, because
-- Postgres pairs an expression index with a query by syntax. Built
-- concurrently, so contact writes are never blocked. The first drop clears an
-- invalid index a failed build left behind.
DROP INDEX CONCURRENTLY IF EXISTS idx_contact_email_mailbox_base;
CREATE INDEX CONCURRENTLY idx_contact_email_mailbox_base ON contact_email ((CASE WHEN regexp_replace(lower(email), '^([^+@]+)\+[^@]*@', '\1@') ~ '^[^@]*@(gmail|googlemail)\.com$' THEN replace(regexp_replace(regexp_replace(lower(email), '^([^+@]+)\+[^@]*@', '\1@'), '@(gmail|googlemail)\.com$', ''), '.', '') || '@gmail.com' ELSE regexp_replace(lower(email), '^([^+@]+)\+[^@]*@', '\1@') END)) WHERE archived_at IS NULL;
DROP INDEX CONCURRENTLY IF EXISTS idx_contact_email_base;
