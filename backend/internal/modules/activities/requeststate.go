// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// requestUnsettledSQL uses the activity alias a. Classification records
// intent; a topic label, a deal outcome, elapsed time and a subsequent email do
// not prove fulfillment. Every reader still composes its own content gate.
const requestUnsettledSQL = `a.kind IN ('email', 'message')
 AND a.direction = 'inbound'
 AND a.archived_at IS NULL AND a.restricted_at IS NULL
 AND NOT EXISTS (SELECT 1 FROM activity settled
   WHERE settled.source_system = '` + EmailRequestTaskSource + `'
     AND settled.source_activity_id = a.id AND settled.is_done)
 AND NOT EXISTS (SELECT 1 FROM activity_sales_state dismissed
   WHERE dismissed.thread_key = a.thread_key AND dismissed.kind = a.kind
     AND dismissed.channel_provider = coalesce(a.channel_provider, ''))`

// Taking a request records intent in the audited source-linked task. It does
// not overwrite the classifier's verdict on somebody else's correspondence.
const acceptedRequestSQL = `EXISTS (SELECT 1 FROM activity accepted
 WHERE accepted.source_system = '` + EmailRequestTaskSource + `'
 AND accepted.source_activity_id = a.id)`

const openRequestReminderSQL = `EXISTS (SELECT 1 FROM activity reminder
 WHERE reminder.source_system = '` + EmailRequestTaskSource + `'
 AND reminder.source_activity_id = a.id AND NOT reminder.is_done
 AND reminder.archived_at IS NULL)`

const outstandingRequestSQL = requestUnsettledSQL + ` AND (a.owed_verdict = 'asks_us' OR ` + acceptedRequestSQL + `)`

// A scheduling/commitment label is enough to keep an unjudged candidate for
// review, never enough to assign it. This also lets the classifier reconcile
// older imported requests instead of aging them out before it reads them.
const requestCandidateSQL = requestUnsettledSQL + ` AND (a.owed_verdict = 'asks_us' OR ` + acceptedRequestSQL + `
 OR (a.owed_verdict IS NULL AND a.capture_label IN ('commitment', 'meeting')))`

func reviewableRequestSQL(asOf string) string {
	return requestUnsettledSQL + ` AND (a.owed_verdict = 'asks_us' OR ` + acceptedRequestSQL + `
 OR (a.owed_verdict IS NULL AND ` + unansweredConversationSQL(asOf) + `))`
}
