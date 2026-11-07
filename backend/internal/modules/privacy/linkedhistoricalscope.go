// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

import (
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

func legacyAuditSubjectQuery(entry legacyAuditEntry) (string, pgx.StrictNamedArgs) {
	args := legacyAuditArgs(entry)
	switch entry.kind {
	case "intro_request":
		return `SELECT erasure.entity_type,erasure.entity_id FROM intro_request record
   JOIN audit_log erasure ON erasure.entity_type='contact' AND erasure.entity_id IN(record.contact_id,record.through_contact_id)
   WHERE record.id=@entity AND ` + legacyPrivacyBoundary + `
   ORDER BY erasure.occurred_at,erasure.id LIMIT 1 FOR UPDATE OF record`, args
	case "dedupe_candidate":
		return `SELECT erasure.entity_type,erasure.entity_id FROM dedupe_candidate record
   JOIN audit_log erasure ON (erasure.entity_type='contact' AND erasure.entity_id IN(record.left_contact_id,record.right_contact_id))
    OR (erasure.entity_type='lead' AND erasure.entity_id IN(record.left_lead_id,record.right_lead_id))
   WHERE record.id=@entity AND ` + legacyPrivacyBoundary + `
   ORDER BY erasure.occurred_at,erasure.id LIMIT 1 FOR UPDATE OF record`, args
	case "approval":
		return legacyApprovalAuditQuery(entry)
	case "data_subject_request":
		return legacyRequestAuditQuery(entry)
	case "deal_room_comment":
		if entry.actorType != "buyer" {
			return "", nil
		}
		buyer, ok := strings.CutPrefix(entry.actorID, "buyer:")
		if !ok {
			return "", nil
		}
		args["buyer"] = buyer
		return `SELECT 'deal'::text,room.deal_id FROM deal_room_participant participant
   JOIN deal_room room ON room.id=participant.room_id
   JOIN audit_log erasure ON erasure.entity_type='deal_room_participant' AND erasure.entity_id=participant.id
   WHERE participant.id::text=@buyer
   AND NOT EXISTS(SELECT 1 FROM deal_room_comment WHERE id=@entity) AND ` + legacyPrivacyBoundary + `
   ORDER BY erasure.occurred_at,erasure.id LIMIT 1 FOR UPDATE OF participant,room`, args
	}
	return "", nil
}

func legacyApprovalAuditQuery(entry legacyAuditEntry) (string, pgx.StrictNamedArgs) {
	args := legacyAuditArgs(entry)
	args["withdrawn"] = subjectWithdrawal
	return `SELECT coalesce(erasure.entity_type,''),coalesce(erasure.entity_id,record.id) FROM approval record
 LEFT JOIN LATERAL(SELECT erasure.entity_type,erasure.entity_id FROM audit_log erasure
  WHERE erasure.entity_type=record.target_entity_type AND erasure.entity_id=record.target_entity_id
  AND ` + legacyPrivacyBoundary + ` ORDER BY erasure.occurred_at,erasure.id LIMIT 1) erasure ON true
 WHERE record.id=@entity AND record.status<>'pending' AND record.proposed_change='{}'::jsonb
 AND coalesce(record.summary,'')='' AND record.summary_parts IS NULL AND record.target_label IS NULL AND record.evidence IS NULL
 AND (erasure.entity_id IS NOT NULL OR (record.status='expired' AND record.decision_reason=@withdrawn))
 FOR UPDATE OF record`, args
}

func legacyRequestAuditQuery(entry legacyAuditEntry) (string, pgx.StrictNamedArgs) {
	return `SELECT coalesce(erasure.entity_type,'data_subject_request'),coalesce(erasure.entity_id,record.id)
 FROM data_subject_request record
 LEFT JOIN LATERAL(SELECT erasure.entity_type,erasure.entity_id FROM audit_log erasure
  WHERE erasure.entity_type='contact' AND (` + legacyPrivacyBoundary + `)
  AND (erasure.entity_id=record.contact_id OR erasure.entity_id::text=record.subject_ref
   OR erasure.evidence->>'reason'='dsr:'||record.id::text
   OR erasure.entity_id::text=(SELECT first.after->>'subject_ref' FROM audit_log first
    WHERE first.entity_type='data_subject_request' AND first.entity_id=record.id AND first.action='create'
    ORDER BY first.occurred_at,first.id LIMIT 1))
  ORDER BY erasure.occurred_at,erasure.id LIMIT 1) erasure ON true
 WHERE record.id=@entity AND NOT record.dispute_open
 AND (erasure.entity_id IS NOT NULL OR (record.subject_ref='erased+'||record.id::text
  AND (record.resolution IS NULL OR record.resolution='erased')))
 FOR UPDATE OF record`, legacyAuditArgs(entry)
}

func legacyAuditArgs(entry legacyAuditEntry) pgx.StrictNamedArgs {
	return pgx.StrictNamedArgs{"entity": entry.entity, "occurred": entry.at, "audit": entry.id, "verbs": storekit.AuditRedactionActions()}
}
