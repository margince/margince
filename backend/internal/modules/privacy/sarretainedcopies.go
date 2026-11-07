// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

import (
	"context"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func sarRetainedCopySections(ctx context.Context, pkg *SARPackage, subject ids.ContactID, emails []string, leads []ids.UUID) ([]sarSection, error) {
	subjects := []any{subject}
	records := []any{subject, leads}
	addresses := []any{loweredAddresses(emails)}
	dedupeArgs := append([]any(nil), records...)
	arg := func(v any) int { dedupeArgs = append(dedupeArgs, v); return len(dedupeArgs) }
	contactScope, err := auth.ScopeClauseFor(ctx, "contact", "sc", arg)
	if err != nil {
		return nil, err
	}
	leadScope, err := auth.ScopeClauseFor(ctx, "lead", "sl", arg)
	if err != nil {
		return nil, err
	}
	contactRows := storekit.SQLf("SELECT sc.id FROM contact sc WHERE sc.id=$%d", len(subjects))
	leadRows := storekit.SQLf("SELECT sl.id FROM lead sl WHERE sl.id=ANY($%d::uuid[])", len(records))
	if contactScope != "" {
		contactRows += " AND (" + contactScope + ")"
	}
	if leadScope != "" {
		leadRows += " AND (" + leadScope + ")"
	}

	subjectArg, leadArg, addressArg := len(subjects), len(records), len(addresses)
	return []sarSection{
		{&pkg.AuditHistory, storekit.SQLf(`SELECT action, entity_type, entity_id, before, after, evidence, occurred_at
		 FROM audit_log WHERE (entity_type='contact' AND entity_id=$%[1]d)
		 OR (entity_type='lead' AND entity_id=ANY($%[2]d::uuid[]))`, subjectArg, leadArg), records},
		{
			&pkg.RetainedEvents, storekit.SQLf(`SELECT envelope->>'type' AS type, envelope->'payload' AS payload, created_at, published_at
		 FROM event_outbox WHERE (envelope->'entity'->>'type'='contact' AND envelope->'entity'->>'id'=$%[1]d::uuid::text)
		 OR (envelope->'entity'->>'type'='lead' AND envelope->'entity'->>'id'=ANY(ARRAY(SELECT id::text FROM unnest($%[2]d::uuid[]) id)))`, subjectArg, leadArg),
			records,
		},
		{&pkg.RoomParticipants, storekit.SQLf(`SELECT id, room_id, full_name, email, capability, created_at, revoked_at
		 FROM deal_room_participant WHERE lower(email)=ANY($%[1]d::text[])`, addressArg), addresses},
		{&pkg.RoomComments, storekit.SQLf(`SELECT c.id, c.room_id, c.thread_id, c.body, c.created_at
		 FROM deal_room_comment c JOIN deal_room_participant p ON p.id=c.author_participant_id
		 WHERE lower(p.email)=ANY($%[1]d::text[])`, addressArg), addresses},
		{&pkg.Introductions, storekit.SQLf(`SELECT id, contact_id, through_contact_id, forwardable_note, value_for_target,
		 internal_reason, decision_reason, status, created_at FROM intro_request
		 WHERE contact_id=$%[1]d`, subjectArg), subjects},
		{&pkg.DuplicateEvidence, `SELECT id, entity_type, confidence, disposition,
          (SELECT coalesce(jsonb_agg(jsonb_build_object('field', item->'field',
            'left_value', CASE WHEN left_contact_id IN (` + contactRows + `) OR left_lead_id IN (` + leadRows + `) THEN item->'left_value' END,
            'right_value', CASE WHEN right_contact_id IN (` + contactRows + `) OR right_lead_id IN (` + leadRows + `) THEN item->'right_value' END)), '[]'::jsonb)
           FROM jsonb_array_elements(CASE WHEN jsonb_typeof(evidence)='array' THEN evidence ELSE '[]'::jsonb END) item) AS evidence
         FROM dedupe_candidate WHERE left_contact_id IN (` + contactRows + `) OR right_contact_id IN (` + contactRows + `)
		 OR left_lead_id IN (` + leadRows + `) OR right_lead_id IN (` + leadRows + `)`, dedupeArgs},
		{&pkg.PlannedCommitments, storekit.SQLf(`SELECT label, help_requested, manager_response, state
		 FROM weekly_plan_commitment WHERE linked_record_type='contact' AND linked_record_id=$%[1]d`, subjectArg), subjects},
		{&pkg.TestMailboxMessages, storekit.SQLf(`SELECT message_id, sent_at, user_id AS withheld_from_mailbox_of
		 FROM capture_test_mailbox_sent WHERE EXISTS (SELECT 1
		 FROM jsonb_array_elements_text(to_addresses || cc_addresses) address
		 WHERE lower(btrim(address))=ANY($%[1]d::text[]))`, addressArg), addresses},
		{&pkg.ThreadVerdicts, storekit.SQLf(`SELECT v.thread_key, v.kind, v.status, v.disposition_reason, v.resolved_at,
		 ARRAY(SELECT address FROM unnest(v.seen_addresses) address
		       WHERE lower(btrim(address))=ANY($%[1]d::text[])) AS subject_addresses
		 FROM capture_thread_verdict v JOIN activity a ON a.id=v.first_activity_id
		 WHERE a.restricted_at IS NULL AND `+sarContentDisclosed+` AND EXISTS (SELECT 1 FROM unnest(v.seen_addresses) address
		 WHERE lower(btrim(address))=ANY($%[1]d::text[]))`, addressArg), addresses},
	}, nil
}
