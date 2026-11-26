-- The write paths already refuse each of these rows. The database now refuses
-- them too, so a row written past those paths cannot grant or price anything.
SET LOCAL lock_timeout = '3s';

-- A rate converts one currency into another, so a pair with itself is the
-- identity, and a zero or negative rate turns every converted figure false.
ALTER TABLE fx_rate
	ADD CONSTRAINT fx_rate_rate_positive CHECK (rate > 0) NOT VALID;
ALTER TABLE fx_rate
	ADD CONSTRAINT fx_rate_pair_distinct CHECK (from_currency <> to_currency) NOT VALID;

-- A credential with no scope grants nothing, and every mint path refuses one.
ALTER TABLE passport
	ADD CONSTRAINT passport_scopes_present CHECK (cardinality(scopes) > 0) NOT VALID;
ALTER TABLE oauth_grant
	ADD CONSTRAINT oauth_grant_scopes_present CHECK (cardinality(scopes) > 0) NOT VALID;
ALTER TABLE oauth_authorization_code
	ADD CONSTRAINT oauth_authorization_code_scopes_present CHECK (cardinality(scopes) > 0) NOT VALID;
-- A code carries the passport scopes plus offline_access, which the exchange
-- turns into the grant's refresh_allowed. backend/gates/arrayvocabulary_test.go
-- holds this list to the Go one.
ALTER TABLE oauth_authorization_code
	ADD CONSTRAINT oauth_authorization_code_scopes_vocabulary
		CHECK (scopes <@ ARRAY['read', 'draft', 'write', 'send', 'enrich', 'offline_access']) NOT VALID;

ALTER TABLE deal_room_invitation
	ADD CONSTRAINT deal_room_invitation_attempt_no_check CHECK (attempt_no > 0) NOT VALID;
