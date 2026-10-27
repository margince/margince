-- Validate the check the migration before this one added NOT VALID. Its own
-- file, so the scan runs under SHARE UPDATE EXCLUSIVE and form writes do not
-- queue behind it. It cannot fail: the rule only got looser.
SET LOCAL lock_timeout = '3s';

ALTER TABLE web_form VALIDATE CONSTRAINT web_form_admission;
