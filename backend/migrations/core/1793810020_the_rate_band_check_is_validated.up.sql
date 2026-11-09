SET LOCAL lock_timeout = '3s';

ALTER TABLE ai_model_rate VALIDATE CONSTRAINT ai_model_rate_long_context_complete_check;
