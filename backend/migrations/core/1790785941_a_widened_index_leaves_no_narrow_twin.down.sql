-- Put each dropped index back exactly as the catalog held it.
SET LOCAL lock_timeout = '3s';

CREATE INDEX idx_booking_page_host ON booking_page USING btree (host_user_id) WHERE (revoked_at IS NULL);
CREATE INDEX communication_suppression_live_contact ON communication_suppression USING btree (contact_id) WHERE ((contact_id IS NOT NULL) AND (revoked_at IS NULL));
CREATE INDEX conversation_claim_activity_ix ON conversation_claim USING btree (source_activity_id) WHERE (archived_at IS NULL);
CREATE INDEX idx_passport_obo ON passport USING btree (on_behalf_of) WHERE (revoked_at IS NULL);
CREATE INDEX idx_project_company_open ON project USING btree (company_id) WHERE ((phase <> 'closed'::text) AND (archived_at IS NULL));
CREATE INDEX idx_company_rel_type_company ON company_relationship_type USING btree (company_id) WHERE (archived_at IS NULL);
