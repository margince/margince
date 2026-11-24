-- The write paths already refuse each of these rows. The database now refuses
-- them too, so a row written past those paths cannot grant or price anything.
SET LOCAL lock_timeout = '3s';

-- A rate converts one currency into another, so a pair with itself is the
-- identity, and a zero or negative rate turns every converted figure false.
ALTER TABLE fx_rate
	ADD CONSTRAINT fx_rate_rate_positive CHECK (rate > 0) NOT VALID;
ALTER TABLE fx_rate
	ADD CONSTRAINT fx_rate_pair_distinct CHECK (from_currency <> to_currency) NOT VALID;

-- The closed scope vocabulary of the identity module. Its test reads these
-- three checks back, so a scope added in Go without a migration fails it.
ALTER TABLE passport
	ADD CONSTRAINT passport_scopes_vocabulary
		CHECK (cardinality(scopes) > 0
			AND scopes <@ ARRAY['read', 'draft', 'write', 'send', 'enrich']) NOT VALID;
ALTER TABLE oauth_grant
	ADD CONSTRAINT oauth_grant_scopes_vocabulary
		CHECK (cardinality(scopes) > 0
			AND scopes <@ ARRAY['read', 'draft', 'write', 'send', 'enrich']) NOT VALID;
-- A code also carries offline_access, which the exchange turns into the
-- grant's refresh_allowed and drops from the scopes it hands on.
ALTER TABLE oauth_authorization_code
	ADD CONSTRAINT oauth_authorization_code_scopes_vocabulary
		CHECK (cardinality(scopes) > 0
			AND scopes <@ ARRAY['read', 'draft', 'write', 'send', 'enrich', 'offline_access']) NOT VALID;

-- The server mints every deal-room credential; no import or connector does.
ALTER TABLE deal_room_invitation
	ADD CONSTRAINT deal_room_invitation_source_check CHECK (source = 'system') NOT VALID;
ALTER TABLE deal_room_session
	ADD CONSTRAINT deal_room_session_source_check CHECK (source = 'system') NOT VALID;
ALTER TABLE deal_room_invitation
	ADD CONSTRAINT deal_room_invitation_attempt_no_check CHECK (attempt_no > 0) NOT VALID;
-- A failed send records the relay's reason, and a sent one records none.
ALTER TABLE deal_room_invitation
	ADD CONSTRAINT deal_room_invitation_failure_stated
		CHECK ((failed_at IS NULL) = (failure_reason IS NULL)) NOT VALID;
