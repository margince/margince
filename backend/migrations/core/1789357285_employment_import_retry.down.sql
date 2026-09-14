SET LOCAL lock_timeout = '3s';

ALTER TABLE contact_provider_claim DROP COLUMN employment_retry_at;
