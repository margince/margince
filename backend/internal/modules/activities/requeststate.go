// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import "github.com/margince/margince/backend/internal/platform/database/storekit"

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
const requestUnsettledSQL = requestOpenSQL + `
 AND ` + notDismissedSQL

// requestOpenSQL is requestUnsettledSQL without the not-sales judgement, which
// owedSQL applies itself so the hidden-backlog reading can relax it alone.
const requestOpenSQL = `a.kind IN ('email', 'message')
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
         AND reopened.is_done = false AND reopened.archived_at IS NULL))`

// notDismissedSQL is false once anybody judged the thread not sales.
const notDismissedSQL = `NOT EXISTS (SELECT 1 FROM activity_sales_state dismissed
   WHERE dismissed.thread_key = a.thread_key AND dismissed.kind = a.kind
     AND dismissed.channel_provider = coalesce(a.channel_provider, ''))`

// Taking a request records intent in the audited source-linked task. It does
// not overwrite the classifier's verdict on somebody else's correspondence.
const acceptedRequestSQL = `EXISTS (SELECT 1 FROM activity accepted
 WHERE accepted.source_system = '` + EmailRequestTaskSource + `'
 AND accepted.source_activity_id = a.id)`

// heldRequestSQL is a request a human kept: an open reminder a human wrote,
// whether they took the request or edited the reminder the system filed. Only
// this outlives the waiting horizon; a classifier verdict does not.
// An id list, not an EXISTS, so the scan reads it from an index. An EXISTS is
// tested on every inbound message ever captured. The list holds 16 bytes per
// open reminder a human wrote, far under any value size limit.
func heldRequestSQL() string {
	return `a.id = ANY (ARRAY(SELECT held.source_activity_id FROM activity held
 WHERE held.archived_at IS NULL AND NOT held.is_done AND held.source_activity_id IS NOT NULL
 AND held.source_system = '` + EmailRequestTaskSource + `'
 AND ` + storekit.HumanWroteActivityClause("held") + `))`
}

const openRequestReminderSQL = `EXISTS (SELECT 1 FROM activity reminder
 WHERE reminder.source_system = '` + EmailRequestTaskSource + `'
 AND reminder.source_activity_id = a.id AND NOT reminder.is_done
 AND reminder.archived_at IS NULL)`

// confirmedRequestSQL is a request the classifier or a human confirmed.
const confirmedRequestSQL = `a.owed_verdict = 'asks_us' OR ` + acceptedRequestSQL

const outstandingRequestSQL = requestUnsettledSQL + ` AND (` + confirmedRequestSQL + `)`

// calendarResponseSQL is an attendee answering an invitation (RFC 5546 REPLY,
// COUNTER, DECLINECOUNTER). Such a message is owed an answer only when
// confirmed: "Declined" waits on nobody, and its meeting label must not say so.
// TestTheCalendarResponseReadersAgree holds it to calendarmethod.IsResponse.
const calendarResponseSQL = `coalesce(a.calendar_method, '') IN ('REPLY', 'COUNTER', 'DECLINECOUNTER')`

// A scheduling/commitment label is enough to keep an unjudged candidate for
// review, never enough to assign it.
const requestIntentSQL = confirmedRequestSQL + `
 OR (a.owed_verdict IS NULL AND a.capture_label IN ('commitment', 'meeting')
     AND NOT ` + calendarResponseSQL + `)`

const requestCandidateSQL = requestUnsettledSQL + ` AND (` + requestIntentSQL + `)`

// reviewableRequestSQL asks a threadless unjudged message for request
// evidence first. The deal card offers these as work, and it applies no
// sender rule, so a hand-logged note or a threadless noreply mail would
// otherwise become a standing obligation on the deal.
func reviewableRequestSQL(asOf string) string {
	return requestUnsettledSQL + ` AND (a.owed_verdict = 'asks_us' OR ` + acceptedRequestSQL + `
 OR (a.owed_verdict IS NULL AND (` + requestCandidateSQL + `
   OR (a.thread_key IS NOT NULL AND NOT ` + calendarResponseSQL + `
       AND NOT ` + answeredSQL("a", asOf) + `))))`
}
