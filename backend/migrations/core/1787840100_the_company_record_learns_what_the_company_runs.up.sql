-- What a company publicly runs, on the company record.
--
-- Four new fact fields under the existing `signal` category, a fifth provenance
-- source that names how they were read, and one signal kind for when one of
-- them changes. Every value describes the legal entity — which system receives
-- its mail, what its website is built with, which services its subdomains
-- reveal, where it hosts — and never a person, which is what keeps this whole
-- lane outside personal-data scope by construction.
--
-- `technology` is deliberately NOT among the new fields: it already exists, and
-- "this company runs Shopware" is one claim about the account whether a site
-- read or a homepage fingerprint observed it. A second field for the second
-- observer would put two answers on the record for one question.
--
-- The new `technical_lookup` source earns an evidence obligation of its own.
-- `org_fact_site_evidence` binds `site_read` alone, so without this a technical
-- fact could be stored with nothing naming the MX host or the certificate that
-- proved it — and the product's answer to "how do you know?" is the whole
-- reason per-field provenance exists here.
--
-- ADD ... NOT VALID then VALIDATE, then swap: the runner wraps each migration in
-- one transaction, so a failure at any statement rolls the whole file back and
-- neither table can be left with no constraint. The two-step also shortens the
-- ACCESS EXCLUSIVE hold — NOT VALID takes it without scanning, and VALIDATE
-- drops to SHARE UPDATE EXCLUSIVE for the pass.
SET LOCAL lock_timeout = '3s';

-- The fact vocabulary gains the four technical fields.
ALTER TABLE organization_fact ADD CONSTRAINT org_fact_field_vocab_v2
    CHECK ((((category = 'company'::text) AND (field IN ('founded_year', 'employee_range', 'phone', 'contact_email', 'location')))
         OR ((category = 'offering'::text) AND (field IN ('service', 'product', 'capability')))
         OR ((category = 'market'::text) AND (field IN ('served_industry', 'company_size', 'geography', 'language')))
         OR ((category = 'signal'::text) AND (field IN ('certification', 'partner', 'named_customer', 'technology',
                                                        'quantified_outcome', 'mail_provider', 'email_security',
                                                        'hosting_provider', 'operated_service')))))
    NOT VALID;

ALTER TABLE organization_fact VALIDATE CONSTRAINT org_fact_field_vocab_v2;

ALTER TABLE organization_fact DROP CONSTRAINT org_fact_field_vocab;

ALTER TABLE organization_fact RENAME CONSTRAINT org_fact_field_vocab_v2 TO org_fact_field_vocab;

-- The provenance vocabulary gains the source that names a technical lookup.
ALTER TABLE organization_fact ADD CONSTRAINT organization_fact_source_check_v2
    CHECK (source IN ('human', 'site_read', 'connector', 'migration', 'technical_lookup'))
    NOT VALID;

ALTER TABLE organization_fact VALIDATE CONSTRAINT organization_fact_source_check_v2;

ALTER TABLE organization_fact DROP CONSTRAINT organization_fact_source_check;

ALTER TABLE organization_fact RENAME CONSTRAINT organization_fact_source_check_v2 TO organization_fact_source_check;

-- A technical fact must name what proved it, the same obligation a site read
-- carries. The evidence is the public record itself: the MX host, the
-- certificate hostname, the matched marker.
ALTER TABLE organization_fact ADD CONSTRAINT org_fact_technical_evidence
    CHECK ((source <> 'technical_lookup'::text)
        OR ((evidence_snippet IS NOT NULL) AND (evidence_snippet <> ''::text)
            AND (source_url IS NOT NULL) AND (source_url <> ''::text)
            AND (retrieved_at IS NOT NULL)))
    NOT VALID;

ALTER TABLE organization_fact VALIDATE CONSTRAINT org_fact_technical_evidence;

-- A change in what a company runs is a company event, on the same surface the
-- newsroom signals reach. Severity is informational: a mail system moving to
-- Microsoft 365 is a reason to call, not a risk to escalate.
ALTER TABLE signal ADD CONSTRAINT signal_kind_check_v3
    CHECK (kind IN ('stalled_deal', 'champion_left', 'reengagement', 'buying_intent',
                    'risk', 'other', 'contract_ended', 'new_opportunity',
                    'commitment_made', 'ghosted_thread', 'project_gone_quiet',
                    'funding', 'leadership_change', 'expansion', 'product_launch',
                    'technical_change'))
    NOT VALID;

ALTER TABLE signal VALIDATE CONSTRAINT signal_kind_check_v3;

ALTER TABLE signal DROP CONSTRAINT signal_kind_check;

ALTER TABLE signal RENAME CONSTRAINT signal_kind_check_v3 TO signal_kind_check;

-- A segment filtering "every account with a webshop" reads the fact table by
-- what the fact SAYS, across every organization. The existing unique index
-- leads with organization_id and cannot serve that scan.
CREATE INDEX idx_org_fact_lookup ON organization_fact (category, field, value_key);

-- What a technical lookup last read for one company, and when to ask again.
--
-- Per LANE rather than per company: the three sources fail independently, and a
-- certificate log being down must never be recorded as "this company has no
-- subdomains". Each lane carries its own success stamp so a failed one changes
-- nothing on the record and is simply re-asked sooner.
CREATE TABLE organization_technical_state (
    organization_id uuid NOT NULL,
    lane text NOT NULL,
    attempts integer DEFAULT 0 NOT NULL,
    last_outcome text,
    last_success_at timestamptz,
    next_attempt_at timestamptz,
    updated_at timestamptz DEFAULT now() NOT NULL,
    CONSTRAINT organization_technical_state_pkey PRIMARY KEY (organization_id, lane),
    CONSTRAINT organization_technical_state_lane_check CHECK (lane IN ('dns', 'certlog', 'homepage')),
    CONSTRAINT organization_technical_state_outcome_check
        CHECK (last_outcome IS NULL OR last_outcome IN ('applied', 'empty', 'failed', 'refused'))
);

ALTER TABLE organization_technical_state
    ADD CONSTRAINT organization_technical_state_organization_id_fkey
    FOREIGN KEY (organization_id) REFERENCES organization(id) ON DELETE CASCADE;

CREATE INDEX idx_org_technical_state_due ON organization_technical_state (next_attempt_at)
    WHERE next_attempt_at IS NOT NULL;

COMMENT ON TABLE organization_technical_state IS
    'Per-lane attempt ledger for technical enrichment: what was last read for this company from which public source, and when to ask again.';

GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE organization_technical_state TO margince_app;

-- What the public sources answered, cached across the whole installation.
--
-- A domain's DNS records and certificate history are the same for every tenant
-- — a domain is a domain — so the cache is installation-global exactly like the
-- geocode cache, and for the same reason: it is MANDATORY rather than an
-- optimisation, because these are shared public services and asking them once
-- per tenant for the same answer is the behaviour that gets an installation
-- blocked.
--
-- It EXPIRES, which is the one way it differs from the geocode cache. A place
-- stays where it is; a mail provider moves, and a permanent cache would make
-- the scheduled refresh unable to ever observe the move it exists to catch.
--
-- `answer` holds only what the classifiers already accepted. A raw certificate
-- hostname can carry a person's name, so nothing raw is written here — the
-- allowlist runs before the cache, not after it.
CREATE TABLE technical_lookup_cache (
    query text NOT NULL,
    record_kind text NOT NULL,
    answer jsonb NOT NULL,
    found boolean NOT NULL,
    fetched_at timestamptz DEFAULT now() NOT NULL,
    expires_at timestamptz NOT NULL,
    CONSTRAINT technical_lookup_cache_pkey PRIMARY KEY (query, record_kind),
    CONSTRAINT technical_lookup_cache_kind_check
        CHECK (record_kind IN ('mx', 'txt', 'dmarc', 'dkim', 'address', 'cname', 'reverse', 'certlog'))
);

CREATE INDEX idx_technical_lookup_cache_expiry ON technical_lookup_cache (expires_at);

COMMENT ON TABLE technical_lookup_cache IS
    'Installation-global, EXPIRING cache of public technical lookups. Holds classified answers only — never a raw certificate hostname, which can carry a personal name.';

GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE technical_lookup_cache TO margince_app;
