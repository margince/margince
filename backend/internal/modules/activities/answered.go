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
)

// answerArm is one kind of evidence that a message was answered: the rows it
// reads, and the column that says when the answer happened.
type answerArm struct {
	from string
	at   string
}

// answerArms are the answers an inbound message can get strictly after it
// arrived and no later than until:
//
//   - our reply on the same thread;
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
	later := func(row string) string {
		return row + `.archived_at IS NULL
	    AND ` + row + `.occurred_at <= ` + until + `
	    AND ` + row + `.occurred_at > ` + inbound + `.occurred_at`
	}
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
	// OFFSET 0 keeps the planner walking from the sender to the mail they were
	// named on: flattened, it scanned every later outbound and ran the subject
	// expression on each.
	return []answerArm{
		threadAnswerArm(inbound, later("answer_thread")),
		{at: "answer_mail.occurred_at", from: `FROM activity_participant answer_asker
	  ` + senderAddresses + `
	  JOIN activity answer_mail ON answer_mail.counterparty_email = answer_address.address
	  WHERE ` + asker + `
	    AND ` + ourMail},
		{at: "answer_mail.occurred_at", from: `FROM activity_participant answer_asker
	  ` + senderAddresses + `
	  CROSS JOIN LATERAL (SELECT answer_mail.* FROM activity_participant answer_told
	     JOIN activity answer_mail ON answer_mail.id = answer_told.activity_id
	     WHERE lower(answer_told.address) = answer_address.address
	       AND answer_told.role IN ('to', 'cc') OFFSET 0) answer_mail
	  WHERE ` + asker + `
	    AND ` + ourMail},
		{at: "answer_touch.occurred_at", from: `FROM activity_participant answer_asker
	  JOIN activity_link answer_link ON answer_link.contact_id = answer_asker.contact_id
	  JOIN activity answer_touch ON answer_touch.id = answer_link.activity_id
	  WHERE ` + asker + `
	    AND ` + touch},
		{at: "answer_touch.occurred_at", from: `FROM activity_participant answer_asker
	  JOIN activity_participant answer_attendee ON answer_attendee.contact_id = answer_asker.contact_id
	  JOIN activity answer_touch ON answer_touch.id = answer_attendee.activity_id
	  WHERE ` + asker + `
	    AND ` + touch},
	}
}

// threadAnswerArm is our reply on the same thread. It reads the reply whoever
// may open it, as the waiting lane always has: a reply on the conversation
// answered the customer whether or not this reader may see it.
func threadAnswerArm(inbound, later string) answerArm {
	return answerArm{at: "answer_thread.occurred_at", from: `FROM activity answer_thread
	  WHERE answer_thread.thread_key = ` + inbound + `.thread_key
	    AND answer_thread.kind = ` + inbound + `.kind
	    AND answer_thread.kind IN ('email', 'message')
	    AND answer_thread.channel_provider IS NOT DISTINCT FROM ` + inbound + `.channel_provider
	    AND answer_thread.direction = 'outbound'
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
// A request (asked of us, accepted by a human, or an unjudged scheduling or
// commitment mail) stays owed through replies until it is completed, settled
// or dismissed: a reply is not proof the request was met. Any other inbound
// is owed while it is the newest inbound on its thread and unanswered. Mail
// from an address no human reads is owed only as a confirmed request.
//
// Two judgements end the obligation and are each counted in
// /worklist/hidden: a human's not-sales call on the thread, and the
// classifier's informs_us verdict, which yields to a human accepting the
// mail as a request. dismissedStillOwed and informsStillOwed are predicates
// OR-ed in front of them; the hidden-backlog reading passes TRUE to count
// what one hides, every other caller passes neverRelaxed.
//
// The newer-inbound walk names its kinds for the index, as answerArms does.
func owedSQL(asOf, dismissedStillOwed, informsStillOwed string) string {
	return `(` + requestOpenSQL + `
	 AND (` + dismissedStillOwed + ` OR ` + notDismissedSQL + `)
	 AND (` + informsStillOwed + ` OR a.owed_verdict IS DISTINCT FROM '` + OwedVerdictInformsUs + `'
	   OR ` + acceptedRequestSQL + `)
	 AND ((` + confirmedRequestSQL + `) OR NOT ` + machineSenderSQL + `)
	 AND ((` + requestIntentSQL + `)
	   OR (NOT EXISTS (SELECT 1 FROM activity newer
	         WHERE newer.thread_key = a.thread_key
	           AND newer.kind = a.kind
	           AND newer.kind IN ('email', 'message')
	           AND newer.channel_provider IS NOT DISTINCT FROM a.channel_provider
	           AND newer.direction = 'inbound'
	           AND newer.archived_at IS NULL
	           AND newer.occurred_at <= ` + asOf + `
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
