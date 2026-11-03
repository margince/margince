// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// Whether an inbound message was answered, and whether a reply is still owed
// on it. Every surface that shows "needs reply" or a waiting row reads these.
//
// Held by: TestOnlyTheAnswerPredicateWalksAThreadForOurReply
// (backend/gates/answerwalk_test.go)

import (
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

func answeredSQL(inbound, until string) string { return storekit.AnsweredSQL(inbound, until) }
func answeredReadableSQL(inbound, until, readable string) string {
	return storekit.AnsweredReadableSQL(inbound, until, readable)
}
func offThreadAnswersSQL(inbound, until string) string {
	return storekit.OffThreadAnswersSQL(inbound, until)
}
func firstAnswerAtSQL(inbound, until string) string { return storekit.FirstAnswerAtSQL(inbound, until) }
func normalisedSubject(column string) string        { return storekit.NormalisedSubjectSQL(column) }

// owedSQL is whether a reply is still owed on the message under alias a, as
// of asOf. The "needs reply" badge is exactly this; the waiting lane is this
// plus its queue rules.
//
// Captured mail requires confirmed intent; a topic label does not prove work.
// Confirmed requests survive replies until fulfilled, settled or dismissed.
// Unclassified manually logged mail retains the newest-unanswered rule.
// Machine senders and calendar responses require confirmed requests too.
//
// Two judgements end the obligation and are each counted in
// /worklist/hidden: a human's not-sales call on the thread, and the
// classifier's informs_us verdict, which yields to a human accepting the
// mail as a request. dismissedStillOwed and informsStillOwed are predicates
// OR-ed in front of them; the hidden-backlog reading passes TRUE to count
// what one hides, every other caller passes neverRelaxed.
//
// unjudgedResponseStillOwed admits unjudged captured mail and calendar responses
// to classification. Only the owed-verdict backlog passes TRUE; pending intent
// must remain judgeable without appearing as an obligation.
//
// The newer-inbound walk names its kinds for the index, as answerArms does.
func owedSQL(asOf, dismissedStillOwed, informsStillOwed, unjudgedResponseStillOwed string) string {
	return `(NOT ` + ImportedHistorySQL + ` AND ` + requestOpenSQL + `
 AND (` + capturedReplyAdmissionSQL(unjudgedResponseStillOwed, informsStillOwed) + `)
	 AND (` + dismissedStillOwed + ` OR ` + notDismissedSQL + `)
	 AND (` + informsStillOwed + ` OR a.owed_verdict IS DISTINCT FROM '` + OwedVerdictInformsUs + `'
	   OR ` + acceptedRequestSQL + `)
	 AND ((` + confirmedRequestSQL + `) OR NOT ` + machineSenderSQL + `)
	 AND ((` + confirmedRequestSQL + `) OR NOT ` + calendarCopySQL + `
	   OR (` + unjudgedResponseStillOwed + ` AND a.owed_verdict IS NULL))
	 AND ((` + requestIntentSQL + `)
	   OR (NOT EXISTS (SELECT 1 FROM activity newer
	         WHERE newer.thread_key = a.thread_key
	           AND newer.kind = a.kind
	           AND newer.kind IN ('email', 'message')
	           AND newer.channel_provider IS NOT DISTINCT FROM a.channel_provider
	           AND newer.direction = 'inbound'
	           AND newer.archived_at IS NULL
	           AND ` + occurredday.BegunSQL("newer", asOf) + `
	           AND (newer.occurred_at, newer.id) > (a.occurred_at, a.id))
	       AND NOT ` + answeredSQL("a", asOf) + `)))`
}

// machineSenderSQL is true when the message came from an address no human
// reads. Deliberately coarse: it removes what nothing could mistake for a
// contact, and the waiting seam's fuller address rule still runs over what
// the lane returns.
const machineSenderSQL = `EXISTS (
	 SELECT 1 FROM activity_participant machine
	  WHERE machine.activity_id = a.id
	    AND machine.role = 'from'
	    AND machine.address ~* '(noreply|no-reply|do-not-reply|donotreply|notification|mailer-daemon)')`

// Captured mail needs an intent judgment before it becomes somebody's work.
// The classifier still reads pending mail; a human can explicitly keep noise.
// A machine verdict stands only while the mail was read in full; a human's
// acceptance or reviewed verdict stands on its own.
func capturedReplyAdmissionSQL(classifying, reviewingInformational string) string {
	human := heldRequestSQL() + ` OR (a.owed_verdict = 'asks_us' AND ` + humanReviewedOwedSQL("a") + `)`
	return `(` + classifying + ` OR (` + reviewingInformational + ` AND a.owed_verdict = 'informs_us') OR (` + human + `) OR (
   a.capture_label IS DISTINCT FROM 'noise'
   AND (NOT starts_with(a.captured_by, '` + string(principal.PrincipalConnector) + `:') OR ` + acceptedRequestSQL + `
     OR (a.owed_verdict = 'asks_us' AND ` + provenance.MailBodyCompleteSQL("a") + `))))`
}
