-- Put each dropped index back exactly as the catalog held it.
SET LOCAL lock_timeout = '3s';

CREATE INDEX idx_ai_feedback_subject ON ai_feedback USING btree (subject_type, subject_id);
CREATE INDEX capture_counterparty_hold_user_idx ON capture_counterparty_hold USING btree (user_id, kind);
CREATE INDEX idx_contact_profile_field ON contact_profile_field USING btree (contact_id);
CREATE INDEX idx_contact_social_contact ON contact_social USING btree (contact_id);
CREATE INDEX idx_dsh_deal ON deal_stage_history USING btree (deal_id, changed_at);
CREATE INDEX idx_forecast_contribution_snapshot ON forecast_contribution USING btree (snapshot_id);
CREATE INDEX idx_list_member_list ON list_member USING btree (list_id);
CREATE INDEX idx_notice_recipient_user ON notice USING btree (recipient_user_id);
CREATE INDEX idx_record_grant_record ON record_grant USING btree (record_type, record_id);
CREATE INDEX idx_taggable_tag ON taggable USING btree (tag_id);
CREATE INDEX idx_team_membership_team ON team_membership USING btree (team_id);
CREATE INDEX voice_corpus_source_profile_fk ON voice_corpus_source USING btree (voice_profile_id);
CREATE INDEX voice_profile_delta_profile_fk ON voice_profile_delta USING btree (voice_profile_id);
CREATE INDEX voice_profile_version_profile_fk ON voice_profile_version USING btree (voice_profile_id);
CREATE INDEX idx_weekly_review_driver_bar ON weekly_review_driver USING btree (weekly_review_id, period_kind, bar);
