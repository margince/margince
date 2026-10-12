-- Put every dropped index back exactly as the catalog held it.
SET LOCAL lock_timeout = '3s';

CREATE INDEX idx_rel_traverse_company ON relationship USING btree (company_id) WHERE (archived_at IS NULL);
CREATE INDEX idx_rel_traverse_contact ON relationship USING btree (contact_id) WHERE (archived_at IS NULL);
CREATE INDEX idx_rel_traverse_deal ON relationship USING btree (deal_id) WHERE (archived_at IS NULL);
CREATE INDEX idx_rel_traverse_project ON relationship USING btree (project_id) WHERE (archived_at IS NULL);
CREATE INDEX idx_rel_company_contacts ON relationship USING btree (company_id) WHERE ((kind = 'employment'::text) AND (archived_at IS NULL));
CREATE INDEX idx_rel_company_projects ON relationship USING btree (company_id) WHERE ((kind = 'project_company'::text) AND (archived_at IS NULL));
CREATE INDEX idx_rel_contact_companies ON relationship USING btree (contact_id) WHERE ((kind = 'employment'::text) AND (archived_at IS NULL));
CREATE INDEX idx_rel_contact_projects ON relationship USING btree (contact_id) WHERE ((kind = 'project_stakeholder'::text) AND (archived_at IS NULL));
CREATE INDEX idx_rel_deal_stakeholders ON relationship USING btree (deal_id) WHERE ((kind = 'deal_stakeholder'::text) AND (archived_at IS NULL));
CREATE INDEX idx_rel_partner_company ON relationship USING btree (company_id) WHERE ((kind = ANY (ARRAY['partner_of'::text, 'referred_by'::text, 'co_sell_with'::text])) AND (archived_at IS NULL));
CREATE INDEX idx_rel_partner_counterparty ON relationship USING btree (counterparty_company_id) WHERE ((kind = ANY (ARRAY['partner_of'::text, 'referred_by'::text, 'co_sell_with'::text])) AND (archived_at IS NULL));
CREATE INDEX idx_rel_project_companies ON relationship USING btree (project_id) WHERE ((kind = 'project_company'::text) AND (archived_at IS NULL));
CREATE INDEX idx_rel_project_stakeholders ON relationship USING btree (project_id) WHERE ((kind = 'project_stakeholder'::text) AND (archived_at IS NULL));
CREATE INDEX idx_rel_stakeholder_deals ON relationship USING btree (contact_id) WHERE ((kind = 'deal_stakeholder'::text) AND (archived_at IS NULL));
CREATE INDEX idx_rel_works_with_contact ON relationship USING btree (contact_id) WHERE ((kind = 'works_with'::text) AND (archived_at IS NULL));
CREATE INDEX idx_rel_works_with_counterparty ON relationship USING btree (counterparty_contact_id) WHERE ((kind = 'works_with'::text) AND (archived_at IS NULL));
