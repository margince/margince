SET LOCAL lock_timeout = '3s';
-- The domain each own-domain check compares, folded in Go by owndomain.Fold:
-- lowercased and IDNA-encoded, so bücher.example and xn--bcher-kva.example are
-- one value. Postgres cannot fold IDNA, so the writers store it and SQL only
-- compares. NULL means no domain: none written yet, an address without one, or
-- on company_domain a public suffix nobody owns. A data task fills older rows.
ALTER TABLE activity_participant ADD COLUMN IF NOT EXISTS address_domain_folded text;
ALTER TABLE company_domain ADD COLUMN IF NOT EXISTS domain_folded text;
ALTER TABLE app_user ADD COLUMN IF NOT EXISTS email_domain_folded text;
