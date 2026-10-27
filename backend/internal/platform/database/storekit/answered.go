// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"fmt"
	"strings"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/mailsubject"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
)

// answerArm is one kind of evidence that a message was answered: the rows it
// reads, and the column that says when the answer happened.
type answerArm struct {
	from string
	at   string
	id   string
}

// Answers follow the source strictly in event time. Capture order cannot break timestamp ties.
// Private off-thread evidence cannot clear another reader's work.
func answerArms(inbound, until string) []answerArm {
	return append([]answerArm{threadAnswerArm(inbound, answerLater("answer_thread", inbound, until))},
		offThreadAnswerArms(inbound, until)...)
}

// answerLater is a row that is live and happened strictly after the inbound
// row and no later than until.
func answerLater(row, inbound, until string) string {
	return fmt.Sprintf(`%[1]s.archived_at IS NULL
	    AND %[3]s
	    AND %[1]s.occurred_at > %[2]s.occurred_at -- occurred_at raw: strictly after the inbound row, an ordering of rows
	`, row, inbound, occurredday.BegunSQL(row, until))
}

// Off-thread mail must be attested and name the sender; Bcc never proves delivery.
// Sender-first lateral probes preserve bounded index plans.
// Predicates must retain idx_activity_answer_mail and idx_activity_answer_touch conditions.
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
	    AND ` + NormalisedSubjectSQL(inbound+".subject") + ` <> ''
	    AND ` + NormalisedSubjectSQL("answer_mail.subject") + ` = ` + NormalisedSubjectSQL(inbound+".subject")
	ourMailIndexed := `answer_mail.kind = ` + inbound + `.kind
	       AND answer_mail.direction = 'outbound'
	       AND answer_mail.counterparty_outbound_attested
	       AND ` + everyoneReads("answer_mail") + `
	       AND ` + later("answer_mail")
	asker := `answer_asker.activity_id = ` + inbound + `.id AND answer_asker.role = 'from'`
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

// Explicit columns keep custom activity fields out of the lateral row shape.
const answerMailColumns = `answer_mail.id, answer_mail.kind, answer_mail.channel_provider,
	answer_mail.direction, answer_mail.counterparty_outbound_attested, answer_mail.restricted_at,
	answer_mail.audience, answer_mail.source_activity_id, answer_mail.source_system, answer_mail.source_id, answer_mail.archived_at,
	answer_mail.occurred_at, answer_mail.occurred_precision, answer_mail.subject`

// Contact-first probes keep the planner from scanning all later calls and meetings.
// OFFSET 0 preserves that join order.
func touchAnswerArm(asker, touch, walk string) answerArm {
	return answerArm{at: "answer_touch.occurred_at", id: "answer_touch.id", from: `FROM activity_participant answer_asker
	  CROSS JOIN LATERAL (` + walk + ` OFFSET 0) answer_walk
	  CROSS JOIN LATERAL (SELECT answer_touch.id, answer_touch.occurred_at FROM activity answer_touch
	     WHERE answer_touch.id = answer_walk.activity_id
	       AND ` + touch + ` OFFSET 0) answer_touch
	  WHERE ` + asker}
}

// A thread can contain different correspondents. Only a reply delivered to this sender counts.
// Same-thread answers retain the waiting lane's established audience semantics.
// Settlement uses ourOutboundInThisThread's narrower attested-counterparty question instead.
func threadAnswerArm(inbound, later string) answerArm {
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

// NormalisedSubjectSQL is a subject with its reply prefixes and outer spaces
// removed and inner runs of space folded, lower-cased. A forward prefix stays,
// so "Fwd: Invoice" never equals "Invoice".
func NormalisedSubjectSQL(column string) string {
	return `lower(btrim(regexp_replace(regexp_replace(coalesce(` + column + `, ''),
	    '` + mailsubject.ReplyPrefixPattern() + `', '', 'i'), '\s+', ' ', 'g')))`
}

// AnsweredSQL is true when the inbound row under the given alias has an
// answer no later than until. It renders no placeholder of its own.
func AnsweredSQL(inbound, until string) string {
	arms := answerArms(inbound, until)
	exists := make([]string, 0, len(arms))
	for _, arm := range arms {
		exists = append(exists, "EXISTS (SELECT 1 "+arm.from+")")
	}
	return "(" + strings.Join(exists, "\n\t OR ") + ")"
}

// AnsweredReadableSQL is AnsweredSQL counting only an answer the caller may
// read. readable is their content clause over the alias readable_answer.
func AnsweredReadableSQL(inbound, until, readable string) string {
	arms := answerArms(inbound, until)
	exists := make([]string, 0, len(arms))
	for _, arm := range arms {
		exists = append(exists, "EXISTS (SELECT 1 FROM (SELECT "+arm.id+" AS id "+arm.from+") answer_found"+
			" JOIN activity readable_answer ON readable_answer.id = answer_found.id WHERE readable_answer.restricted_at IS NULL AND "+readable+")")
	}
	return "(" + strings.Join(exists, " OR ") + ")"
}

// OffThreadAnswersSQL supplies the existing settlement engine with off-thread evidence.
func OffThreadAnswersSQL(inbound, until string) string {
	arms := offThreadAnswerArms(inbound, until)
	selects := make([]string, 0, len(arms))
	for _, arm := range arms {
		selects = append(selects, "SELECT "+arm.id+" AS id, "+arm.at+" AS occurred_at "+arm.from)
	}
	return strings.Join(selects, "\n\t UNION ")
}

// FirstAnswerAtSQL is when the inbound row got its first answer, or NULL when
// it has none. The response time is measured to it.
func FirstAnswerAtSQL(inbound, until string) string {
	arms := answerArms(inbound, until)
	firsts := make([]string, 0, len(arms))
	for _, arm := range arms {
		firsts = append(firsts, "(SELECT min("+arm.at+") "+arm.from+")")
	}
	return "LEAST(" + strings.Join(firsts, ",\n\t ") + ")"
}
