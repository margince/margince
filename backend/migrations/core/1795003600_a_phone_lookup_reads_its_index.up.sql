-- The phone lane of the duplicate check matches stored E.164 numbers exactly,
-- and nothing indexed the number, so every contact write read all of
-- contact_phone. The partial predicate is the one the lookup filters on.
SET LOCAL lock_timeout = '3s';
CREATE INDEX IF NOT EXISTS idx_contact_phone_live_phone
  ON contact_phone (phone)
  WHERE archived_at IS NULL;
