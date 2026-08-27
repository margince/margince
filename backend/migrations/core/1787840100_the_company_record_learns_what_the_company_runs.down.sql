-- Same bound as the up: the constraint swaps take an ACCESS EXCLUSIVE lock on
-- tables the enrichment and signal passes write on every run.
--
-- Rows carrying the retired vocabulary would fail the narrowed constraints, so
-- they go first. The technical facts are DELETED rather than archived, unlike
-- the signals below: a fact row is the current state of a claim and has no
-- archived form, and every one of them is re-derivable from public sources by
-- the next run. A signal is history somebody may already have read and acted
-- on, so it is retired to `other` and archived instead.
SET LOCAL lock_timeout = '3s';

DELETE FROM organization_fact
 WHERE source = 'technical_lookup'
    OR (category = 'signal'
        AND field IN ('mail_provider', 'email_security', 'hosting_provider', 'operated_service'));

UPDATE signal
   SET kind = 'other', archived_at = coalesce(archived_at, now())
 WHERE kind = 'technical_change';

ALTER TABLE signal ADD CONSTRAINT signal_kind_check_v2
    CHECK (kind IN ('stalled_deal', 'champion_left', 'reengagement', 'buying_intent',
                    'risk', 'other', 'contract_ended', 'new_opportunity',
                    'commitment_made', 'ghosted_thread', 'project_gone_quiet',
                    'funding', 'leadership_change', 'expansion', 'product_launch'))
    NOT VALID;

ALTER TABLE signal VALIDATE CONSTRAINT signal_kind_check_v2;

ALTER TABLE signal DROP CONSTRAINT signal_kind_check;

ALTER TABLE signal RENAME CONSTRAINT signal_kind_check_v2 TO signal_kind_check;

ALTER TABLE organization_fact DROP CONSTRAINT org_fact_technical_evidence;

ALTER TABLE organization_fact ADD CONSTRAINT organization_fact_source_check_v1
    CHECK (source IN ('human', 'site_read', 'connector', 'migration'))
    NOT VALID;

ALTER TABLE organization_fact VALIDATE CONSTRAINT organization_fact_source_check_v1;

ALTER TABLE organization_fact DROP CONSTRAINT organization_fact_source_check;

ALTER TABLE organization_fact RENAME CONSTRAINT organization_fact_source_check_v1 TO organization_fact_source_check;

ALTER TABLE organization_fact ADD CONSTRAINT org_fact_field_vocab_v1
    CHECK ((((category = 'company'::text) AND (field IN ('founded_year', 'employee_range', 'phone', 'contact_email', 'location')))
         OR ((category = 'offering'::text) AND (field IN ('service', 'product', 'capability')))
         OR ((category = 'market'::text) AND (field IN ('served_industry', 'company_size', 'geography', 'language')))
         OR ((category = 'signal'::text) AND (field IN ('certification', 'partner', 'named_customer', 'technology', 'quantified_outcome')))))
    NOT VALID;

ALTER TABLE organization_fact VALIDATE CONSTRAINT org_fact_field_vocab_v1;

ALTER TABLE organization_fact DROP CONSTRAINT org_fact_field_vocab;

ALTER TABLE organization_fact RENAME CONSTRAINT org_fact_field_vocab_v1 TO org_fact_field_vocab;

DROP INDEX idx_org_fact_lookup;

DROP TABLE technical_lookup_cache;

DROP TABLE organization_technical_state;
