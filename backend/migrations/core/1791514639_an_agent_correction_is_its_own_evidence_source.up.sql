-- An agent's correction of a company fact says an agent made it.
--
-- Before this, the evidence write path stamped source = 'human' on any row it
-- touched, while captured_by came from the authenticated principal. An agent
-- correction therefore read as human-confirmed while the capture column said a
-- machine had written it. The two disagreed in the direction that claims more
-- authority than it holds.
--
-- NOT VALID, and validated in the file beside this one. The widening cannot
-- fail for an existing row, but a plain ADD CONSTRAINT still scans the whole
-- table under ACCESS EXCLUSIVE, and every write waits for the scan.
--
-- Rows that agents already wrote as 'human' are a data fix rather than a
-- backfill here: a migration that rewrote them would decide, inside DDL, which
-- historical rows an agent authored.
SET LOCAL lock_timeout = '3s';

ALTER TABLE company_fact DROP CONSTRAINT company_fact_source_check;
ALTER TABLE company_fact ADD CONSTRAINT company_fact_source_check
  CHECK (source IN ('human', 'agent', 'site_read', 'connector', 'migration', 'technical_lookup')) NOT VALID;

ALTER TABLE company_profile_field DROP CONSTRAINT company_profile_field_source_check;
ALTER TABLE company_profile_field ADD CONSTRAINT company_profile_field_source_check
  CHECK (source IN ('human', 'agent', 'site_read', 'connector', 'migration')) NOT VALID;
