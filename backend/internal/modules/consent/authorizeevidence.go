// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// The record reads behind a resolution: is this recipient on the thread that
// was started, and is there a live opportunity they are a stakeholder on.
//
// Each one answers about ONE recipient, because the decision is per recipient.
// A message to four people is four questions, and a reader asking why one of
// them was refused is owed an answer about that person rather than about the
// send.

import (
	"context"
	"fmt"

	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// subjectRef names who a decision is about: a person or a lead, never both.
//
// It carries the KIND as well as the id because the two arms live in different
// columns everywhere they are read, and a bare uuid would let a lead id be
// compared against a person column and quietly match nothing — a refusal that
// looks exactly like an absent relationship.
type subjectRef struct {
	Kind string
	ID   string
	// Address is the mailbox the message is going to. A thread participant is
	// recorded by person OR by bare address, so an address that never resolved
	// to a record can still be shown to have been on the thread.
	Address string
}

// repliesToTheSubject reports whether this recipient took part in the thread
// the anchor message belongs to.
//
// THREAD, not message. A rep answering the third mail in an exchange anchors on
// that mail, and the recipient may have written only the first — asking whether
// they were on this one message would refuse a perfectly ordinary reply. The
// thread is the unit of correspondence, so the thread is what is asked about.
//
// The subject must have SENT something into it, not merely appeared in it.
// Being copied on a message somebody else wrote is not the subject initiating
// correspondence, and treating it as such would let anyone create a lawful
// basis for writing to a third party by putting them in Cc.
func repliesToTheSubject(ctx context.Context, tx pgx.Tx, anchor ids.UUID, subject subjectRef) (bool, error) {
	var found bool
	err := tx.QueryRow(ctx, `
		WITH anchor AS (
			SELECT thread_key FROM activity WHERE id = $4 AND archived_at IS NULL
		)
		SELECT EXISTS (
			SELECT 1
			  FROM activity a
			  JOIN anchor ON anchor.thread_key IS NOT NULL
			                 AND a.thread_key = anchor.thread_key
			  JOIN activity_participant p ON p.activity_id = a.id
			 WHERE a.direction = 'inbound'
			   AND a.archived_at IS NULL
			   -- Authorship, from the shared spelling: the subject WROTE into
			   -- this thread, and a recipient who was merely copied has
			   -- initiated nothing.
			   AND `+authorIsTheSubject+`
		)`, subject.Kind, subject.ID, subject.Address, anchor).Scan(&found)
	if err != nil {
		return false, fmt.Errorf("consent: read the thread this message answers: %w", err)
	}
	return found, nil
}

// wroteToUsWithin reports whether the subject SENT us something inside the
// window, on any thread.
//
// It shares repliesToTheSubject's participant test rather than spelling a
// second one, and the reason is the whole finding it exists to answer: an
// earlier version of this asked activity_link instead, which is a FILING link
// with no author concept at all. That reads "some inbound activity is filed
// under this person", and since a caller may post an activity with
// direction=inbound and a link to any contact they can read, it let anybody
// manufacture their own evidence for writing to anybody.
//
// The author test is the same one the thread arm uses: role 'from', and the
// bare-address arm only for a participant capture never resolved to a record.
// Two writers of one invariant either share a helper or say why they do not —
// this is the helper.
func wroteToUsWithin(ctx context.Context, tx pgx.Tx, subject subjectRef, since time.Time) (bool, error) {
	var found bool
	err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			  FROM activity a
			  JOIN activity_participant p ON p.activity_id = a.id
			 WHERE a.direction = 'inbound'
			   AND a.archived_at IS NULL
			   AND a.occurred_at >= $4
			   AND `+authorIsTheSubject+`
		)`, subject.Kind, subject.ID, subject.Address, since).Scan(&found)
	if err != nil {
		return false, fmt.Errorf("consent: read what this person sent us: %w", err)
	}
	return found, nil
}

// authorIsTheSubject is the ONE spelling of "this recipient wrote this
// message", shared by the thread arm and the window arm.
//
// Held by: TestBeingCopiedOnAThreadIsNotWritingIntoIt (authorizeresolve_integration_test.go)
// and TestAFiledActivityIsNotSomethingThePersonWrote (authorizevalidators_integration_test.go)
// — the first fails if the thread arm stops requiring authorship, the second if
// the window arm goes back to reading a filing link.
//
// $1 subject kind, $2 subject id, $3 address. The placeholders are fixed so
// both callers bind the same three in the same order; a caller adding its own
// must number above them.
const authorIsTheSubject = `(
			         p.role = 'from'
			         AND (
			               ($1 = 'person' AND p.person_id = $2::uuid)
			            OR ($3 <> '' AND p.person_id IS NULL AND lower(p.address) = lower($3))
			             )
			       )`

// liveDealInLinks reports whether one of the records this message is filed
// under is an OPEN deal the recipient is a stakeholder on.
//
// Both halves are required and neither is sufficient. A live deal the recipient
// has nothing to do with does not make them writable-to — that is the shape
// that turns one opportunity into a licence to mail everyone at the company.
// And a stakeholder relationship on a closed deal is history: the opportunity
// that justified the follow-up is over.
func liveDealInLinks(ctx context.Context, tx pgx.Tx, links []ids.UUID, subject subjectRef) (bool, error) {
	if len(links) == 0 || subject.Kind != entityPerson {
		// A lead is never a deal stakeholder: rel_stakeholder_shape requires a
		// person_id, so asking would compare a lead id against a person column
		// and always answer no. Said here rather than discovered as an empty
		// result, because the two look identical from the caller.
		return false, nil
	}
	var found bool
	err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			  FROM deal d
			  JOIN relationship r ON r.deal_id = d.id
			 WHERE d.id = ANY($1)
			   AND d.status = 'open'
			   AND d.archived_at IS NULL
			   AND r.kind = 'deal_stakeholder'
			   AND r.person_id = $2::uuid
			   -- BOTH, and they are different facts. ended_at is a business
			   -- date somebody types; archived_at is how the edge is actually
			   -- removed — every delete and every cascade (person archive,
			   -- merge, deal archive) writes archived_at and leaves ended_at
			   -- alone. Checking only ended_at would let a stakeholder who was
			   -- REMOVED from the deal keep supporting mail about it.
			   AND r.ended_at IS NULL
			   AND r.archived_at IS NULL
		)`, links, subject.ID).Scan(&found)
	if err != nil {
		return false, fmt.Errorf("consent: read the opportunity this message follows up: %w", err)
	}
	return found, nil
}

// askedToBeContacted reports whether this person's acquisition evidence records
// them ASKING to hear from us, inside the window.
//
// TWO KINDS ONLY, and the narrowness is the rule. Where a contact came from is
// provenance; a purchased list and a public source say nothing about what the
// person wanted. Only requested_quote_or_meeting and in_person_permission ARE a
// request, and they exist so a rep who logged "they asked me for a quote at the
// fair" is not asked to record the same fact twice in a second table.
//
// BOUNDED IN BOTH DIRECTIONS. occurred_at is caller-stated — the column exists
// so an import can land today carrying a business card collected last year —
// so a future date is not a memory of anything and must not authorize a send
// now. The same bound and the same reason as RecordQualifyingEvent's, with the
// same small allowance for a client clock that runs fast.
func askedToBeContacted(ctx context.Context, tx pgx.Tx, subject subjectRef, since time.Time) (bool, error) {
	if subject.Kind != entityPerson {
		// person_acquisition_evidence is keyed on person_id; a lead holds none.
		return false, nil
	}
	var found bool
	err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM person_acquisition_evidence e
			 WHERE e.person_id = $1::uuid
			   AND e.kind IN ('requested_quote_or_meeting', 'in_person_permission')
			   AND coalesce(e.occurred_at, e.captured_at) >= $2
			   AND coalesce(e.occurred_at, e.captured_at) <= $3
		)`, subject.ID, since, time.Now().Add(clockSkewAllowance)).Scan(&found)
	if err != nil {
		return false, fmt.Errorf("consent: read what this person asked for: %w", err)
	}
	return found, nil
}
