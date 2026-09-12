SET LOCAL lock_timeout = '3s';

DROP TABLE IF EXISTS activity_review_response;
DROP TABLE IF EXISTS activity_review_template;

ALTER TABLE deal_stage_history DROP CONSTRAINT deal_stage_history_semantic_at_change_check;
ALTER TABLE deal_stage_history DROP COLUMN semantic_at_change;
ALTER TABLE deal_stage_history DROP CONSTRAINT deal_stage_history_deal_id_id_key;
