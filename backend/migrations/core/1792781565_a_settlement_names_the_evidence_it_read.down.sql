-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '2s';

DROP TRIGGER IF EXISTS settlement_evidence_share ON contact_email;
DROP TRIGGER IF EXISTS settlement_evidence_share ON activity_link;
DROP TRIGGER IF EXISTS settlement_evidence_share ON activity_participant;
DROP TRIGGER IF EXISTS settlement_evidence_share ON activity;
DROP FUNCTION IF EXISTS settlement_evidence_share_contact_email();
DROP FUNCTION IF EXISTS settlement_evidence_share_link();
DROP FUNCTION IF EXISTS settlement_evidence_share_participant();
DROP FUNCTION IF EXISTS settlement_evidence_share_activity();
DROP FUNCTION IF EXISTS settlement_evidence_contact_email_keys(contact_email);
DROP FUNCTION IF EXISTS settlement_evidence_participant_keys(activity_participant);
DROP FUNCTION IF EXISTS settlement_evidence_activity_keys(activity);
DROP FUNCTION IF EXISTS settlement_evidence_share(bigint[]);
DROP FUNCTION IF EXISTS settlement_evidence_key(text, text);
ALTER TABLE activity_request_settlement DROP COLUMN IF EXISTS revalidation_requested_at;
ALTER TABLE activity_request_settlement DROP COLUMN IF EXISTS judged_evidence;
