// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

import "github.com/margince/margince/backend/internal/shared/kernel/provenance"

// LiveLeadInquiryClause excludes imported history and requires an incoming
// message before automatic intake work can claim a first-response obligation.
func LiveLeadInquiryClause(alias string) string {
	return "NOT starts_with(COALESCE(" + alias + ".source_system, ''), '" + provenance.ReservedSourceSystemPrefix + "') AND EXISTS (SELECT 1 FROM activity_link incoming" +
		" JOIN activity request ON request.id = incoming.activity_id WHERE incoming.lead_id = " + alias + ".id" +
		" AND request.kind = 'email' AND request.direction = 'inbound'" +
		" AND request.archived_at IS NULL AND request.restricted_at IS NULL" +
		" AND NOT starts_with(COALESCE(request.source_system, ''), '" + provenance.ReservedSourceSystemPrefix + "'))"
}

var importedFollowupTaskIDsSQL = `SELECT emitted.envelope->'entity'->>'id'
 FROM workflow_run automatic
 JOIN lead imported ON imported.id::text = automatic.planned #>> '{0,Target,ID}'
 JOIN event_outbox emitted ON emitted.envelope->'trace'->>'causation_id' = automatic.trigger_event::text
 WHERE automatic.handler = 'route_lead'
   AND starts_with(COALESCE(imported.source_system, ''), '` + provenance.ReservedSourceSystemPrefix + `')
   AND emitted.envelope->>'type' = 'activity.captured'
   AND emitted.envelope->'entity'->>'id' IS NOT NULL`

// LiveIntakeTaskClause removes untouched automatic intake work, including
// legacy import follow-ups identified by their causal trace. Edited tasks stay.
func LiveIntakeTaskClause(alias string) string {
	return "(" + alias + ".version > 1 OR ((COALESCE(" + alias + ".source_system, '') <> 'lead_sla'" +
		" OR EXISTS (SELECT 1 FROM activity_link intake JOIN lead ld ON ld.id = intake.lead_id" +
		" WHERE intake.activity_id = " + alias + ".id AND " + LiveLeadInquiryClause("ld") + ")) AND " +
		alias + ".id::text NOT IN (" + importedFollowupTaskIDsSQL + ")))"
}
