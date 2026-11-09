SET LOCAL lock_timeout = '3s';

-- The records, by type and id, and the field names a proposal's plan touched
-- when it was decided or expired. It holds no record value, so a later reader's
-- sight and field masks can be checked again without keeping what they guard.
ALTER TABLE approval ADD COLUMN plan_footprint jsonb;
