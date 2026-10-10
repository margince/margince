// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

// The Art. 15 chapter for what this installation CONCLUDED about the subject's
// own correspondence.
//
// Its own file rather than more rows in sarsections.go, because it answers a
// different question from every other chapter there. The rest of the gather
// list exports what the subject GAVE us or what we recorded about handling
// them; this exports readings we made of their words — a verdict on what a
// reply meant, and a sentence naming what we still owe them. Art. 15 owes those
// on a footing of their own: a subject is entitled to know not only what they
// wrote but what we decided it meant, and which classifier decided it.

import "github.com/margince/margince/backend/internal/shared/kernel/ids"

// sarSubjectMessages is the one set of activities all three sections read
// from. It holds what links to the subject, what they took part in, and mail
// that named one of their addresses. A link alone misses a message filed only
// to their company or deal, or to nobody at all.
//
// It keeps rows shared with another contact or under a legal hold. Erasure
// skips those to protect them; Art. 15 still owes the subject access to them.
const sarSubjectMessages = `
	SELECT l.activity_id FROM activity_link l WHERE l.contact_id = $1
	UNION
	SELECT ap.activity_id FROM activity_participant ap
	  JOIN activity pa ON pa.id = ap.activity_id
	 WHERE ap.contact_id = $1
	    OR ap.address = ANY($2)
	    OR (ap.channel_user_id IS NOT NULL AND (pa.channel_provider, ap.channel_user_id) IN (
	        SELECT provider, channel_user_id FROM contact_channel_identity WHERE contact_id = $1))
	UNION
	SELECT m.id FROM activity m WHERE m.counterparty_email = ANY($2)
	UNION
	SELECT d.activity_id FROM comms_outbound d
	 WHERE d.activity_id IS NOT NULL AND EXISTS (
	       SELECT 1 FROM jsonb_array_elements_text(
	                      d.recipients || d.cc || coalesce(d.bcc, '[]'::jsonb)) AS addr
	        WHERE lower(addr) = ANY($2))`

// sarJudgementSections gather the conclusions. Each section names both
// arguments, because args REPLACES the default rather than adding to it: one
// that passed only the contact would leave $2 unbound.
func sarJudgementSections(pkg *SARPackage, contactID ids.ContactID, emails []string) []sarSection {
	subject := []any{contactID, emails}
	return []sarSection{
		// What we concluded the subject's replies MEANT, and who concluded it.
		// The history is exported rather than the activity's current column
		// alone, because the corrections are the half a subject cannot see any
		// other way: a rate that was moved by a rep re-judging their message is
		// a decision made about them, and the standing verdict conceals that it
		// ever happened.
		//
		// The verdict itself is NOT withheld for a limited message, where the
		// subject line beside it is. The rule those CASE arms implement is that
		// one seat's private mail must not be republished to the workspace
		// through an export; this export goes to the SUBJECT, who wrote the
		// message being judged, so withholding our conclusion about their own
		// words would hide the very holding Art. 15 asks about.
		{&pkg.ReplyJudgements, `SELECT h.verdict, h.decided_by, h.is_human, h.decided_at,
		          h.activity_id, a.occurred_at
		   FROM activity_reply_verdict_history h
		   JOIN activity a ON a.id = h.activity_id
		   WHERE h.activity_id IN (` + sarSubjectMessages + `)`, subject},
		// And what we concluded our own reply DID about what they asked. The
		// `remaining` sentence travels: it is the one place the package can say
		// we recorded an outstanding obligation TO the subject, and a settlement
		// exported without it would report that we judged their request and
		// withhold what we judged. The reply we read through is named by its
		// date rather than only its id, for the reason the handoff sections give
		// about labels — a bare uuid tells the subject nothing about which
		// message of ours the conclusion rests on.
		{
			&pkg.RequestSettlements, `SELECT s.verdict, s.remaining, s.due_at, s.confidence,
		          s.decided_by, s.decided_at, s.request_activity_id,
		          asked.occurred_at AS asked_at, answered.occurred_at AS answered_at
		   FROM activity_request_settlement s
		   JOIN activity asked ON asked.id = s.request_activity_id
		   LEFT JOIN activity answered ON answered.id = s.judged_through_activity_id
		   WHERE s.request_activity_id IN (` + sarSubjectMessages + `)`,
			subject,
		},
		// What a deal's stage ledger read out of the subject's messages. The
		// criterion is named by its label, because a bare id tells the subject
		// nothing about what the claim was for. A refuted claim travels too: a
		// conclusion withdrawn later was still held about them.
		{&pkg.StageEvidence, `SELECT ev.snippet, ev.commitment, ev.met, ev.confidence,
		          ev.author_side, ev.extracted_by, ev.observed_at, ev.refuted_at,
		          ev.source_id AS activity_id, c.label AS criterion
		   FROM deal_stage_evidence ev
		   JOIN stage_exit_criterion c ON c.id = ev.criterion_id
		   WHERE ev.source_type = 'activity' AND ev.source_id IN (` + sarSubjectMessages + `)`, subject},
	}
}
