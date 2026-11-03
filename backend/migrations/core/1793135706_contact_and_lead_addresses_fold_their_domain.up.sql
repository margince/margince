SET LOCAL lock_timeout = '3s';
-- The folded domain of a contact's and a lead's address, by owndomain.Fold:
-- lowercased and IDNA-encoded, so bücher.example and xn--bcher-kva.example
-- are one value. Postgres cannot fold IDNA, so the writers store it and the
-- company and duplicate matchers compare it. NULL means no domain, or none
-- written yet; a data task fills older rows.
ALTER TABLE contact_email ADD COLUMN IF NOT EXISTS email_domain_folded text;
ALTER TABLE lead ADD COLUMN IF NOT EXISTS email_domain_folded text;
