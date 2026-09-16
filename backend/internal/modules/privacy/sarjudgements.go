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

// sarJudgementSections gather the conclusions, reached by the same two arms the
// Art. 17 erasure reaches them by.
//
// BOTH ARMS, and the second is the one that matters. A request carrying no
// contact link is the ordinary shape since ADR-0072 — a deferred or
// still-unsure sender produces activities linked to nobody — and the settlement
// pass judges one happily, because its candidate read requires no link either.
// An export walking links alone would hand the subject a package silently
// missing the judgements about exactly that mail, while reporting itself
// complete. The erasure and the export have to give one answer to "which of
// these are this subject's", which is why both spell it with the same two
// selectors.
func sarJudgementSections(pkg *SARPackage, contactID ids.ContactID, emails []string) []sarSection {
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
		   WHERE h.activity_id IN (
		         SELECT l.activity_id FROM activity_link l WHERE l.contact_id = $1)`, nil},
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
		   WHERE s.request_activity_id IN (` + subjectOnlyActivities + `)
		      OR s.request_activity_id IN (` + unlinkedSubjectMail + `)`,
			// BOTH arguments, because args REPLACES the default rather than
			// adding to it: a section naming its own list owns the whole list,
			// and one that passed only the addresses would leave $1 holding an
			// array where the selectors want the contact.
			[]any{contactID, emails},
		},
	}
}
