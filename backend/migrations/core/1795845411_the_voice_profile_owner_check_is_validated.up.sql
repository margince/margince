-- No writer ever set a team scope, so every voice profile meets the narrower
-- owner check the previous file added.
SET LOCAL lock_timeout = '3s';

ALTER TABLE voice_profile VALIDATE CONSTRAINT voice_profile_scope_owner_check;
