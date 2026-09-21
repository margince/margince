// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// requestUnsettledSQL uses the activity alias a. Classification records intent;
// a topic label, a deal outcome and elapsed time do not prove fulfillment.
// Every reader still composes its own content gate.
//
// A REPLY SETTLES NOTHING BY ITSELF, and the middle arm is not an exception to
// that. "Thanks, I will check" is a reply and discharges nothing, so the mere
// existence of a later outbound message is deliberately not tested here — it
// was not before this arm existed and it is not now. What the arm reads is a
// JUDGEMENT over the thread, recorded only where a model read our own words and
// concluded they answered what was asked.
//
// It yields to a task somebody reopened. A human picking the work back up
// outranks what the pass decided about it, and without that inner clause a
// reopened reminder would sit on a request this predicate reports as settled —
// the queue and the task list disagreeing about the same obligation.
const requestUnsettledSQL = `a.kind IN ('email', 'message')
 AND a.direction = 'inbound'
 AND a.archived_at IS NULL AND a.restricted_at IS NULL
 AND NOT EXISTS (SELECT 1 FROM activity settled
   WHERE settled.source_system = '` + EmailRequestTaskSource + `'
     AND settled.source_activity_id = a.id AND settled.is_done)
 AND NOT EXISTS (SELECT 1 FROM activity_request_settlement answered
   WHERE answered.request_activity_id = a.id
     AND answered.verdict = '` + RequestSettled + `'
     AND NOT EXISTS (SELECT 1 FROM activity reopened
       WHERE reopened.source_system = '` + EmailRequestTaskSource + `'
         AND reopened.source_activity_id = a.id
         AND reopened.is_done = false AND reopened.archived_at IS NULL))
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
