ALTER TABLE ai_model_rate DROP CONSTRAINT IF EXISTS ai_model_rate_lane_check;

ALTER TABLE ai_model_rate DROP COLUMN IF EXISTS lane;
