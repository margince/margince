SET LOCAL lock_timeout = '3s';

ALTER TABLE contact_provider_claim ADD COLUMN employment_retry_at timestamptz;
