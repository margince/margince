-- Puts the old spellings back. Lossy in one way worth naming: where the up half
-- folded an old row's counts into a new one, the two are now one row and the
-- split cannot be recovered -- the rename returns, the arithmetic does not.
SET LOCAL lock_timeout = '3s';

UPDATE approval SET kind = 'quota_release' WHERE kind = 'volume_release';
UPDATE approval_autonomy_policy SET kind = 'quota_release' WHERE kind = 'volume_release';
