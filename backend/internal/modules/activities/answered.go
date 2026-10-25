// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// Whether an inbound message was answered, and whether a reply is still owed
// on it. Every surface that shows "needs reply" or a waiting row reads these.
//
// Held by: TestOnlyTheAnswerPredicateWalksAThreadForOurReply
// (backend/gates/answerwalk_test.go)

import (
	"strings"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/mailsubject"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

// answerArm is one kind of evidence that a message was answered: the rows it
// reads, and the column that says when the answer happened.
type answerArm struct {
	from string
	at   string
	id   string
}

// answerArms are the answers an inbound message can get strictly after it
// arrived and no later than until:
//
//   - our reply on the same thread, sent to the sender: its counterparty or a
//     To/Cc row is their address or one of their contact's;
//   - our sent mail to the sender with the same subject once reply prefixes
//     are stripped. The sender is their address or their contact, so any of
//     the contact's addresses counts; the mail must name them as its
//     counterparty or on a To/Cc row (Bcc is not an answer they can see);
//   - a logged call or a held meeting with the sender's contact.
//
// The subject match is read here only. Capture never joins threads on a
// subject, because two "Re: Invoice" mails from two senders are two
// conversations; here the sender keeps them apart.
//
// Strictly later, never by id: mail carries second precision, and an id says
// when a row was captured, not which message came first. A tie stays owed.
//
// Evidence off the thread counts only when the whole workspace may read it,
// and mail only when the provider filed it as sent by us. A colleague's
// private mail, call or meeting must not clear another seat's row, since the
// row disappearing would disclose that the private evidence exists; and a
// captured message whose From merely names our mailbox proves nothing.
func answerArms(inbound, until string) []answerArm {
	return append([]answerArm{threadAnswerArm(inbound, answerLater("answer_thread", inbound, until))},
		offThreadAnswerArms(inbound, until)...)
}

// answerLater is a row that is live and happened strictly after the inbound
// row and no later than until.
func answerLater(row, inbound, until string) string {
	return row + `.archived_at IS NULL
	    AND ` + occurredday.BegunSQL(row, until) + `
	    AND ` + row + `.occurred_at > ` + inbound + `.occurred_at`
}

// offThreadAnswerArms are the answers of answerArms that sit outside the
// inbound row's thread.
func offThreadAnswerArms(inbound, until string) []answerArm {
	later := func(row string) string { return answerLater(row, inbound, until) }
	everyoneReads := func(row string) string {
		return row + `.restricted_at IS NULL` + auth.AudienceWorkspaceOnly(row)
	}
	ourMail := `answer_mail.kind = ` + inbound + `.kind
	    AND answer_mail.channel_provider IS NOT DISTINCT FROM ` + inbound + `.channel_provider
	    AND answer_mail.direction = 'outbound'
	    AND answer_mail.counterparty_outbound_attested
	    AND ` + everyoneReads("answer_mail") + `
	    AND ` + later("answer_mail") + `
	    AND ` + normalisedSubject(inbound+".subject") + ` <> ''
	    AND ` + normalisedSubject("answer_mail.subject") + ` = ` + normalisedSubject(inbound+".subject")
	// The clauses of idx_activity_answer_mail, inside the lateral so the index
	// qualifies with the address as its leading key.
	ourMailIndexed := `answer_mail.kind = ` + inbound + `.kind
	       AND answer_mail.direction = 'outbound'
	       AND answer_mail.counterparty_outbound_attested
	       AND ` + everyoneReads("answer_mail") + `
	       AND ` + later("answer_mail")
	asker := `answer_asker.activity_id = ` + inbound + `.id AND answer_asker.role = 'from'`
	// The sender's own address and every live address of their contact.
	senderAddresses := `CROSS JOIN LATERAL (
	    SELECT lower(btrim(answer_asker.address)) AS address
	    UNION
	    SELECT lower(btrim(answer_known.email)) FROM contact_email answer_known
	     WHERE answer_known.contact_id = answer_asker.contact_id AND answer_known.archived_at IS NULL
	  ) answer_address`
	touch := `(answer_touch.kind = '` + string(crmcontracts.ActivityKindCall) + `'
	      OR (answer_touch.kind = '` + string(crmcontracts.ActivityKindMeeting) + `'
	        AND answer_touch.meeting_status = '` + string(crmcontracts.ActivityMeetingStatusHeld) + `'))
	    AND ` + everyoneReads("answer_touch") + `
	    AND ` + later("answer_touch")
	// The kind list in the thread walks restates what the equality already
	// implies, so the planner can prove idx_activity_thread_reply_seek applies.
	// The touch and ourMail predicates are likewise the predicates of
	// idx_activity_answer_touch and idx_activity_answer_mail, clause for clause:
	// narrow either fragment freely, but a clause dropped from one stops the
	// planner proving its index applies, and the arm walks every activity of
	// the contact again.
	// OFFSET 0 keeps the planner walking from the sender to the mail they were
	// named on, in both same-subject arms: flattened, the planner ranged
	// idx_activity_answer_mail on kind and time alone and applied the address
	// afterwards, so every message read every later attested mail of the
	// installation and ran the subject expression on each.
	return []answerArm{
		{at: "answer_mail.occurred_at", id: "answer_mail.id", from: `FROM activity_participant answer_asker
	  ` + senderAddresses + `
	  CROSS JOIN LATERAL (SELECT ` + answerMailColumns + ` FROM activity answer_mail
	     WHERE answer_mail.counterparty_email = answer_address.address
	       AND ` + ourMailIndexed + ` OFFSET 0) answer_mail
	  WHERE ` + asker + `
	    AND ` + ourMail},
		{at: "answer_mail.occurred_at", id: "answer_mail.id", from: `FROM activity_participant answer_asker
	  ` + senderAddresses + `
	  CROSS JOIN LATERAL (SELECT ` + answerMailColumns + ` FROM activity_participant answer_told
	     JOIN activity answer_mail ON answer_mail.id = answer_told.activity_id
	     WHERE lower(answer_told.address) = answer_address.address
	       AND answer_told.role IN ('to', 'cc') OFFSET 0) answer_mail
	  WHERE ` + asker + `
	    AND ` + ourMail},
		touchAnswerArm(asker, touch, `SELECT answer_link.activity_id FROM activity_link answer_link
	     WHERE answer_link.contact_id = answer_asker.contact_id`),
		touchAnswerArm(asker, touch, `SELECT answer_attendee.activity_id FROM activity_participant answer_attendee
	     WHERE answer_attendee.contact_id = answer_asker.contact_id`),
	}
}

// answerMailColumns are the columns the mail arms read off a lateral row, named
// because an administrator can add a custom-field column to activity at runtime.
const answerMailColumns = `answer_mail.id, answer_mail.kind, answer_mail.channel_provider,
	answer_mail.direction, answer_mail.counterparty_outbound_attested, answer_mail.restricted_at,
	answer_mail.audience, answer_mail.source_activity_id, answer_mail.archived_at,
	answer_mail.occurred_at, answer_mail.subject`

// touchAnswerArm is a logged call or held meeting among the activities walk
// names for the sender's contact.
//
// Walked from the contact and in that order: walk reads the contact's own links
// or attendances through idx_alink_contact or idx_aparticipant_contact, and
// each activity it names is probed by id for the touch. The second lateral
// takes the walked id as its parameter, so no plan can start from the
// activity side; and OFFSET 0 keeps both from being flattened. Flattened, the
// planner ranged idx_activity_answer_touch on occurred_at alone, so every
// inbound read every later call and meeting of the workspace and joined each
// back to the sender: a cost of messages times activity, where this one is
// bounded by the contact's own history.
func touchAnswerArm(asker, touch, walk string) answerArm {
	return answerArm{at: "answer_touch.occurred_at", id: "answer_touch.id", from: `FROM activity_participant answer_asker
	  CROSS JOIN LATERAL (` + walk + ` OFFSET 0) answer_walk
	  CROSS JOIN LATERAL (SELECT answer_touch.id, answer_touch.occurred_at FROM activity answer_touch
	     WHERE answer_touch.id = answer_walk.activity_id
	       AND ` + touch + ` OFFSET 0) answer_touch
	  WHERE ` + asker}
}

// threadAnswerArm is our reply on the same thread. It reads the reply whoever
// may open it, as the waiting lane always has: a reply on the conversation
// answered the customer whether or not this reader may see it.
//
// The reply must be sent to the sender. One thread can hold our mail to a
// second correspondent, and that answers nobody else on it. A Bcc copy is not
// an answer either, even when a colleague's copy took it as its counterparty.
// A message with no sender address, a chat, binds on the thread alone.
//
// ourOutboundInThisThread asks a narrower question, for settlement evidence:
// attested mail to one stored counterparty. It decides no badge, so it keeps
// neither the To/Cc rows nor the Bcc test read here.
func threadAnswerArm(inbound, later string) answerArm {
	// The off-thread arms' senderAddresses starts from the `from` row for its
	// index plan. Here the folded counterparty counts too: an inbound row may
	// have no `from` row.
	sender := `SELECT ` + inbound + `.counterparty_email
	     UNION
	    SELECT lower(btrim(answer_known.email)) FROM activity_participant answer_asker
	      JOIN contact_email answer_known ON answer_known.contact_id = answer_asker.contact_id
	       AND answer_known.archived_at IS NULL
	     WHERE answer_asker.activity_id = ` + inbound + `.id AND answer_asker.role = 'from'`
	return answerArm{at: "answer_thread.occurred_at", id: "answer_thread.id", from: `FROM activity answer_thread
	  WHERE answer_thread.thread_key = ` + inbound + `.thread_key
	    AND answer_thread.kind = ` + inbound + `.kind
	    AND answer_thread.kind IN ('email', 'message')
	    AND answer_thread.channel_provider IS NOT DISTINCT FROM ` + inbound + `.channel_provider
	    AND answer_thread.direction = 'outbound'
	    AND (` + inbound + `.counterparty_email IS NULL
	      OR (answer_thread.counterparty_email IN (` + sender + `)
	        AND NOT EXISTS (SELECT 1 FROM activity_participant answer_blind
	             WHERE answer_blind.activity_id = answer_thread.id AND answer_blind.role = 'bcc'
	               AND lower(btrim(answer_blind.address)) = answer_thread.counterparty_email))
	      OR EXISTS (SELECT 1 FROM activity_participant answer_told
	           WHERE answer_told.activity_id = answer_thread.id AND answer_told.role IN ('to', 'cc')
	             AND lower(btrim(answer_told.address)) IN (` + sender + `)))
	    AND ` + later}
}

// normalisedSubject is a subject with its reply prefixes and outer spaces
// removed and inner runs of space folded, lower-cased. A forward prefix stays,
// so "Fwd: Invoice" never equals "Invoice".
func normalisedSubject(column string) string {
	return `lower(btrim(regexp_replace(regexp_replace(coalesce(` + column + `, ''),
	    '` + mailsubject.ReplyPrefixPattern() + `', '', 'i'), '\s+', ' ', 'g')))`
}

// answeredSQL is true when the inbound row under the given alias has an
// answer no later than until. It renders no placeholder of its own.
func answeredSQL(inbound, until string) string {
	arms := answerArms(inbound, until)
	exists := make([]string, 0, len(arms))
	for _, arm := range arms {
		exists = append(exists, "EXISTS (SELECT 1 "+arm.from+")")
	}
	return "(" + strings.Join(exists, "\n\t OR ") + ")"
}

// offThreadAnswersSQL selects, as (id, occurred_at), every answer to the
// inbound row under alias that sits outside its thread: our attested mail to
// the sender with the same subject, and a logged call or held meeting with
// their contact. Each arm carries its own audience, attestation and
// strictly-later rules.
func offThreadAnswersSQL(inbound, until string) string {
	arms := offThreadAnswerArms(inbound, until)
	selects := make([]string, 0, len(arms))
	for _, arm := range arms {
		selects = append(selects, "SELECT "+arm.id+" AS id, "+arm.at+" AS occurred_at "+arm.from)
	}
	return strings.Join(selects, "\n\t UNION ")
}

// firstAnswerAtSQL is when the inbound row got its first answer, or NULL when
// it has none. The response time is measured to it.
func firstAnswerAtSQL(inbound, until string) string {
	arms := answerArms(inbound, until)
	firsts := make([]string, 0, len(arms))
	for _, arm := range arms {
		firsts = append(firsts, "(SELECT min("+arm.at+") "+arm.from+")")
	}
	return "LEAST(" + strings.Join(firsts, ",\n\t ") + ")"
}

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
