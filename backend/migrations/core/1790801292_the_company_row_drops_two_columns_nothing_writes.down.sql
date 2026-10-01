-- Put both columns back exactly as the catalog held them, defaults and all.
SET LOCAL lock_timeout = '3s';

ALTER TABLE company
    ADD COLUMN classification text NOT NULL DEFAULT 'prospect',
    ADD COLUMN relevance smallint,
    ADD CONSTRAINT company_classification_check
        CHECK (classification IN ('prospect', 'customer', 'agency', 'reseller', 'tech_vendor',
                                  'platform', 'partner', 'competitor', 'other')),
    ADD CONSTRAINT company_relevance_check
        CHECK (relevance IS NULL OR (relevance >= 0 AND relevance <= 100));

CREATE INDEX idx_company_class ON company USING btree (classification) WHERE (archived_at IS NULL);
