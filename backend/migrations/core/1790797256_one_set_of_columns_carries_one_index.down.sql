-- Put each dropped constraint and index back exactly as the catalog held it.
SET LOCAL lock_timeout = '3s';

ALTER TABLE ai_call ADD CONSTRAINT uq_ai_call_ws_id UNIQUE (id);
ALTER TABLE capture_connection ADD CONSTRAINT uq_capture_connection_ws_id UNIQUE (id);
ALTER TABLE oauth_grant ADD CONSTRAINT oauth_grant_ws_id_key UNIQUE (id);
ALTER TABLE offer ADD CONSTRAINT uq_offer_ws_id UNIQUE (id);
ALTER TABLE offer_template ADD CONSTRAINT uq_offer_template_ws_id UNIQUE (id);
ALTER TABLE passport ADD CONSTRAINT uq_passport_ws_id UNIQUE (id);
ALTER TABLE product ADD CONSTRAINT uq_product_ws_id UNIQUE (id);
ALTER TABLE project ADD CONSTRAINT uq_project_ws_id UNIQUE (id);
ALTER TABLE site_read ADD CONSTRAINT uq_site_read_ws_id UNIQUE (id);

ALTER TABLE oauth_client ADD CONSTRAINT oauth_client_unique UNIQUE (client_id);
ALTER TABLE voice_profile_version ADD CONSTRAINT uq_voice_profile_version_profile_number UNIQUE (voice_profile_id, profile_version);

CREATE INDEX idx_brief_item_run ON brief_item USING btree (brief_run_id, rank);
CREATE INDEX deal_correction_by_audit ON deal_correction USING btree (audit_log_id);
CREATE INDEX idx_fx_rate_lookup ON fx_rate USING btree (from_currency, to_currency, rate_date);
CREATE INDEX idx_oli_offer ON offer_line_item USING btree (offer_id, "position");
CREATE INDEX idx_weekly_review_learning_review ON weekly_review_learning USING btree (weekly_review_id, "position");
