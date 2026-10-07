-- Put the check back present and unvalidated. Postgres has no ALTER that
-- un-validates, so it is dropped and re-added NOT VALID, without a scan.
SET LOCAL lock_timeout = '3s';

ALTER TABLE deal
    DROP CONSTRAINT deal_source_author_needs_a_source,
    ADD CONSTRAINT deal_source_author_needs_a_source CHECK (
        (source_author_id IS NULL AND source_author_name IS NULL)
        OR source_system IS NOT NULL) NOT VALID;
