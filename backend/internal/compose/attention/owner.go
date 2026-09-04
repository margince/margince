// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// Who answers for a row, said by the producer that raised it.
//
// RESPONSIBILITY IS NOT VISIBILITY, and the whole of this file is that
// distinction. A rep can read a colleague's deal and owes nothing on it; a
// notice addressed to somebody else is unreadable and is still theirs. The two
// inferences that were available here — the reader can see it, so it is theirs;
// the row survived a `mine` filter, so it is theirs — are both wrong in the
// same direction, and both would fill a rep's morning with work they do not
// owe.
//
// So a producer STATES it. Silence is not "unassigned": it is a producer that
// has not answered, which the census catches, and which would otherwise reach
// a reader as a confident claim that nobody owns the row.

import (
	"context"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ownerKind is WHICH WAY a producer knows who owns its rows.
//
// Three answers, and the third is the one that makes this a producer statement
// rather than a downstream guess. A lane that reads a named owner column says
// so; a lane whose rows nobody has taken says so; a lane whose QUERY is bound
// to the acting user says that, and means it — the row is theirs because the
// read could not have returned anybody else's, which is a fact about the lane
// and not about who happens to be looking.
type ownerKind int

const (
	// ownerUnstated is the zero value, and it is not an answer. A row carrying
	// it reached a page from a lane that never said, which the census fails on.
	ownerUnstated ownerKind = iota
	// ownerNamed: this lane resolved a person.
	ownerNamed
	// ownerNobody: this lane's rows are genuinely unheld.
	ownerNobody
	// ownerTheReader: this lane's read is bound to the acting user.
	ownerTheReader
	// ownerFromTheDeal: this row's owner arrives with the deal facts, after
	// classification. A real answer — the lane knows WHERE the answer comes
	// from — rather than the silence the census refuses.
	ownerFromTheDeal
)

// ownerRef is the producer's answer, before the reader is known.
//
// It carries a KIND rather than a resolved id because the classifiers are pure
// functions of the day — classifyDay takes no principal, deliberately, and
// threading one through fifteen of them to write the same id fifteen times
// would put the reader's identity inside a pure ranking. The one place that
// knows the reader resolves it, once, on the page.
type ownerRef struct {
	kind ownerKind
	// user is who answers, for ownerNamed.
	user ids.UUID
}

// ownedBy is a producer naming the person who answers for a row.
func ownedBy(user ids.UUID) ownerRef { return ownerRef{kind: ownerNamed, user: user} }

// unassigned is a producer saying nobody has taken this yet.
//
// A real answer rather than an absence, which is why it is spelled. Work
// nobody owns is what the unassigned scope is for, and a lane that means it
// says so here rather than by staying quiet.
func unassigned() ownerRef { return ownerRef{kind: ownerNobody} }

// ownedByWhoeverIsReading is the answer for the intrinsically per-user sources.
//
// A notice is addressed to one person, a mailbox belongs to one person, a
// promise was made by one person, an approved action failed for the person who
// approved it. Those reads are bound to the acting user INSIDE the modules that
// own them — `mineOnly`'s own comment says so — so the row is the reader's by
// construction of the read.
//
// This is NOT "the reader can see it, so it is theirs". That inference is
// available two lines away and is wrong: a rep can read a colleague's deal and
// owes nothing on it. What this says is narrower and true — the query that
// produced this row took the acting user as a parameter, so no other person's
// row could have come back.
func ownedByWhoeverIsReading() ownerRef { return ownerRef{kind: ownerTheReader} }

// deferredToTheDeal is a lane whose rows are owned through their deal.
//
// The at-risk and brief lanes classify before dealfacts fills OwnerId onto the
// wire, so they cannot name the person here — but they know exactly where the
// answer comes from, and saying so is a statement rather than a silence. The
// wire step reads the deal the facts pass attached.
func deferredToTheDeal() ownerRef { return ownerRef{kind: ownerFromTheDeal} }

// ownerOnTheWire is the field a client draws, with the label still to resolve.
//
// The DEAL's owner is consulted only where the lane itself did not answer, and
// the order matters for the reason answersTo documents: a waiting row absorbs a
// drifting deal's facts for display between two passes, so asking the deal
// first can answer differently on the second pass than on the first, and a
// customer lands on neither person's queue.
func ownerOnTheWire(row ranked, reader ids.UUID) *crmcontracts.WorklistOwner {
	switch row.ownerRef.kind {
	case ownerNamed:
		return &crmcontracts.WorklistOwner{
			Kind: crmcontracts.WorklistOwnerUser,
			Id:   idPtr(row.ownerRef.user),
		}
	case ownerNobody:
		return &crmcontracts.WorklistOwner{Kind: crmcontracts.WorklistOwnerUnassigned}
	case ownerFromTheDeal:
		// The facts pass has run by now, so the deal carries its owner. A deal
		// the caller may not read arrives without figures — the same refusal
		// shape the rest of the response uses — and a row whose owner could not
		// be read says nothing rather than claiming nobody owns it.
		if row.item.Deal != nil && row.item.Deal.OwnerId != nil {
			return &crmcontracts.WorklistOwner{
				Kind: crmcontracts.WorklistOwnerUser,
				Id:   row.item.Deal.OwnerId,
			}
		}
		return nil
	case ownerTheReader:
		// No human behind the call is not "unassigned": an agent reading the
		// queue has no personal lane, and saying nobody owns these rows would
		// be a claim about the work rather than about the caller.
		if reader.IsZero() {
			return nil
		}
		return &crmcontracts.WorklistOwner{
			Kind: crmcontracts.WorklistOwnerUser,
			Id:   idPtr(reader),
		}
	default:
		// A producer that never answered. Reported as no owner field at all
		// rather than as `unassigned`: the census fails on this, and until it
		// does a reader is better served by a row that says nothing about
		// ownership than by one that confidently says nobody owns it.
		return nil
	}
}

// readerOf is who is asking, for the lanes whose rows are theirs by
// construction. Zero where nothing human is behind the call.
func readerOf(ctx context.Context) ids.UUID {
	actor, ok := principal.Actor(ctx)
	if !ok {
		return ids.UUID{}
	}
	return actor.UserID
}

// idPtr is the wire's optional-uuid shape.
func idPtr(user ids.UUID) *openapi_types.UUID {
	out := openapi_types.UUID(user)
	return &out
}

// ownerFrom is a lane that read an owner COLUMN, where the absence of a value
// is itself the answer.
//
// Tasks, leads and waiting customers all work this way: the query returns the
// owner it found, and finding none means nobody has taken the row rather than
// meaning the lane declined to say. Spelled once so the three answer a zero
// alike.
//
// Held by: TestEveryProducerStatesAnOwner (owner_test.go), which reads every
// lane's answer, and TestATasksOwnerAgreesWithItsOwnReason, which holds one of
// the three to the same fact spelled in its own reasons.
func ownerFrom(user ids.UUID) ownerRef {
	if user.IsZero() {
		return unassigned()
	}
	return ownedBy(user)
}

// ownerFromAssignee is a lane that read an assignee COLUMN on the wire item.
//
// The task lane states the same fact twice — as a `unassigned` reason a reader
// sees, and as the owner a client draws — so both read this one function. Two
// spellings of "nobody has taken it" would eventually disagree, and the row
// would say one thing in its reasons and another beside them.
func ownerFromAssignee(assignee *openapi_types.UUID) ownerRef {
	if assignee == nil {
		return unassigned()
	}
	return ownedBy(ids.UUID(*assignee))
}

// nameTheOwners fills in the display name beside each owner id.
//
// THROUGH THE ROSTER THE TEAM BOARD ALREADY READS, rather than a second reader
// of app_user. LiveTeammatesOfCaller answers live human seats sharing a live
// team with the caller — which is exactly the set whose names this caller may
// see — so the label carries the same visibility rule the board does, and a
// name the reader could not otherwise reach never arrives through this field.
// A second query with its own scope clause would be a second answer to "whose
// name may this person read", and the two would drift.
//
// A missing name is left ABSENT rather than filled with the id: the contract
// says a client draws the row without a name in that case, and an id on screen
// is the defect the label exists to prevent. The reader's own rows are the
// common case for this — a caller on no team has no roster — and a row that
// says "you" by carrying the reader's own id needs no name to be legible.
//
// An UNBOUND seam names nobody, and an error is not fatal here: an owner id
// with no name still tells a client who answers for the row, while failing the
// whole page over a display name would take the queue away from a reader whose
// work is on it.
func (s *Service) nameTheOwners(ctx context.Context, queue []crmcontracts.WorklistItem) {
	if s.teammates == nil || !anyOwnerNeedsAName(queue) {
		return
	}
	roster, _, err := s.teammates.LiveTeammatesOfCaller(ctx)
	if err != nil {
		return
	}
	names := make(map[ids.UUID]string, len(roster))
	for _, member := range roster {
		names[member.UserID] = member.DisplayName
	}
	for i := range queue {
		owner := queue[i].Owner
		if owner == nil || owner.Id == nil {
			continue
		}
		if name, known := names[ids.UUID(*owner.Id)]; known && name != "" {
			owner.Label = &name
		}
	}
}

// anyOwnerNeedsAName reports whether the roster read is worth making.
//
// A page of the reader's own work names nobody else, and reading a roster to
// label nothing is a query per page for no reader's benefit.
func anyOwnerNeedsAName(queue []crmcontracts.WorklistItem) bool {
	for _, item := range queue {
		if item.Owner != nil && item.Owner.Id != nil {
			return true
		}
	}
	return false
}
