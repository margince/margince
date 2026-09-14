SET LOCAL lock_timeout = '3s';
ALTER TABLE contact_provider_claim ADD COLUMN employment_retry_count integer NOT NULL DEFAULT 0;
ALTER TABLE contact_provider_claim ADD COLUMN employment_processing_error text;
