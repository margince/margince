-- A token expires after it was created, and nothing said so.
--
-- expires_at is on thirteen tables and no CHECK anywhere mentioned it. Five of
-- them hand out authority: a passport, an oauth grant, an authorization code, a
-- deal-room session and a deal-room invitation are all bearer credentials. A
-- row whose expiry precedes its creation has a lifetime nobody can reason
-- about, and every reader asking "is this still valid" gets an answer that
-- depends on which column it trusted.
--
-- THESE VALIDATIONS CAN FAIL, and that is the outcome to want: a stored
-- credential that expired before it existed is one somebody should look at
-- rather than one to admit quietly.
--
-- The creation column is whichever of created_at, issued_at and fetched_at the
-- table carries: consent_doi_token and confirm_token issue, and
-- technical_lookup_cache fetches.
--
-- MFA_CHALLENGE_SPENT IS NOT HERE. Its expires_at is the CHALLENGE's expiry,
-- copied in so the row can be reaped once the signature stops verifying. The
-- table keeps no creation stamp at all, because it is a nonce ledger and that
-- instant is the only thing it needs.
--
-- APPROVAL IS NOT HERE, and the reason is in approvals/staging.go: superseding
-- a pending approval writes `expires_at = now() - interval '1 day'` so the row
-- reads as expired under the service clock as well as the database one, which
-- may trail it by ordinary clock skew. An approval superseded an hour after it
-- was raised therefore expires a day before it was created, by design. Bounding
-- that column would refuse supersession. The gate beside this migration waives
-- it and carries the reason.
SET LOCAL lock_timeout = '3s';

ALTER TABLE agent_task
	ADD CONSTRAINT agent_task_expires_after_creation
		CHECK (expires_at > created_at) NOT VALID;
ALTER TABLE auth_token
	ADD CONSTRAINT auth_token_expires_after_creation
		CHECK (expires_at > created_at) NOT VALID;
ALTER TABLE deal_room
	ADD CONSTRAINT deal_room_expires_after_creation
		CHECK (expires_at > created_at) NOT VALID;
ALTER TABLE deal_room_invitation
	ADD CONSTRAINT deal_room_invitation_expires_after_creation
		CHECK (expires_at > created_at) NOT VALID;
ALTER TABLE deal_room_session
	ADD CONSTRAINT deal_room_session_expires_after_creation
		CHECK (expires_at > created_at) NOT VALID;
ALTER TABLE oauth_authorization_code
	ADD CONSTRAINT oauth_authorization_code_expires_after_creation
		CHECK (expires_at > created_at) NOT VALID;
ALTER TABLE oauth_refresh_token
	ADD CONSTRAINT oauth_refresh_token_expires_after_creation
		CHECK (expires_at > created_at) NOT VALID;
ALTER TABLE passport
	ADD CONSTRAINT passport_expires_after_creation
		CHECK (expires_at > created_at) NOT VALID;
ALTER TABLE preference_token
	ADD CONSTRAINT preference_token_expires_after_creation
		CHECK (expires_at > created_at) NOT VALID;
ALTER TABLE record_grant
	ADD CONSTRAINT record_grant_expires_after_creation
		CHECK (expires_at > created_at) NOT VALID;
ALTER TABLE session
	ADD CONSTRAINT session_expires_after_creation
		CHECK (expires_at > created_at) NOT VALID;

ALTER TABLE consent_doi_token
	ADD CONSTRAINT consent_doi_token_expires_after_issue
		CHECK (expires_at > issued_at) NOT VALID;

ALTER TABLE confirm_token
	ADD CONSTRAINT confirm_token_expires_after_creation
		CHECK (expires_at > issued_at) NOT VALID;

ALTER TABLE data_task_review
	ADD CONSTRAINT data_task_review_expires_after_creation
		CHECK (expires_at > created_at) NOT VALID;

ALTER TABLE meeting_proposal
	ADD CONSTRAINT meeting_proposal_expires_after_creation
		CHECK (expires_at > created_at) NOT VALID;

ALTER TABLE technical_lookup_cache
	ADD CONSTRAINT technical_lookup_cache_expires_after_creation
		CHECK (expires_at > fetched_at) NOT VALID;

ALTER TABLE withdrawal_credential
	ADD CONSTRAINT withdrawal_credential_expires_after_creation
		CHECK (expires_at > issued_at) NOT VALID;
