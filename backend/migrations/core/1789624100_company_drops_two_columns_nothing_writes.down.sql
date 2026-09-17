-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '2s';

-- Restores the shape, not the data: every row held the default, so a default
-- is the honest thing to restore. This direction exists for a rollback that
-- has not yet run in production, not as a way back from one that has.
ALTER TABLE company
    ADD COLUMN classification text DEFAULT 'prospect'::text NOT NULL,
    ADD COLUMN relevance smallint,
    ADD CONSTRAINT company_classification_check CHECK (classification IN
        ('prospect', 'customer', 'agency', 'reseller', 'tech_vendor',
         'platform', 'partner', 'competitor', 'other')),
    ADD CONSTRAINT company_relevance_check CHECK
        ((relevance IS NULL) OR ((relevance >= 0) AND (relevance <= 100)));

CREATE INDEX idx_company_class ON company USING btree (classification)
    WHERE (archived_at IS NULL);
