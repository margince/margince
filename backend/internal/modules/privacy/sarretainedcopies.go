// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

import "github.com/margince/margince/backend/internal/shared/kernel/ids"

func sarRetainedCopySections(pkg *SARPackage, subject ids.ContactID, emails []string, leads []ids.UUID) []sarSection {
	return []sarSection{
		{&pkg.AuditHistory, `SELECT action, entity_type, entity_id, before, after, evidence, occurred_at
		 FROM audit_log WHERE (entity_type='contact' AND entity_id=$1)
		 OR (entity_type='lead' AND entity_id=ANY($2::uuid[]))`, []any{subject, leads}},
		{&pkg.RetainedEvents, `SELECT envelope->>'type' AS type, envelope->'payload' AS payload, created_at, published_at
		 FROM event_outbox WHERE (envelope->'entity'->>'type'='contact' AND envelope->'entity'->>'id'=$1::uuid::text)
		 OR (envelope->'entity'->>'type'='lead' AND envelope->'entity'->>'id'=ANY(ARRAY(SELECT id::text FROM unnest($2::uuid[]) id)))`,
			[]any{subject, leads}},
		{&pkg.RoomParticipants, `SELECT id, room_id, full_name, email, capability, created_at, revoked_at
		 FROM deal_room_participant WHERE lower(email)=ANY($1::text[])`, []any{loweredAddresses(emails)}},
		{&pkg.RoomComments, `SELECT c.id, c.room_id, c.thread_id, c.body, c.created_at
		 FROM deal_room_comment c JOIN deal_room_participant p ON p.id=c.author_participant_id
		 WHERE lower(p.email)=ANY($1::text[])`, []any{loweredAddresses(emails)}},
		{&pkg.Introductions, `SELECT id, contact_id, through_contact_id, forwardable_note, value_for_target,
		 internal_reason, decision_reason, status, created_at FROM intro_request
		 WHERE contact_id=$1 OR through_contact_id=$1`, nil},
		{&pkg.DuplicateEvidence, `SELECT id, entity_type, confidence, evidence, disposition
		 FROM dedupe_candidate WHERE left_contact_id=$1 OR right_contact_id=$1
		 OR left_lead_id=ANY($2::uuid[]) OR right_lead_id=ANY($2::uuid[])`, []any{subject, leads}},
		{&pkg.PlannedCommitments, `SELECT label, help_requested, manager_response, state
		 FROM weekly_plan_commitment WHERE linked_record_type='contact' AND linked_record_id=$1`, nil},
		{&pkg.TestMailboxMessages, `SELECT message_id, sent_at, user_id AS withheld_from_mailbox_of
		 FROM capture_test_mailbox_sent WHERE EXISTS (SELECT 1
		 FROM jsonb_array_elements_text(to_addresses || cc_addresses) address
		 WHERE lower(btrim(address))=ANY($1::text[]))`, []any{loweredAddresses(emails)}},
		{&pkg.ThreadVerdicts, `SELECT v.thread_key, v.kind, v.status, v.disposition_reason, v.resolved_at,
		 ARRAY(SELECT address FROM unnest(v.seen_addresses) address
		       WHERE lower(btrim(address))=ANY($1::text[])) AS subject_addresses
		 FROM capture_thread_verdict v JOIN activity a ON a.id=v.first_activity_id
		 WHERE a.restricted_at IS NULL AND ` + sarContentDisclosed + ` AND EXISTS (SELECT 1 FROM unnest(v.seen_addresses) address
		 WHERE lower(btrim(address))=ANY($1::text[]))`, []any{loweredAddresses(emails)}},
	}
}
