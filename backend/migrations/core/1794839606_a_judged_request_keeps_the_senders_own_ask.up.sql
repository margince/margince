SET LOCAL lock_timeout = '3s';

-- The sender's own sentence for what a request asks, copied word for word by
-- the triage that judged it. The reminder filed for the request quotes it, so
-- it lives beside the verdict and goes when a later verdict keeps none.
ALTER TABLE activity ADD COLUMN owed_ask text;
