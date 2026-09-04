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
	"fmt"

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
	// ownerUnreadable: this lane HAS an owner and could not read it.
	//
	// A real answer, and a different one from every kind above. The waiting
	// lane qualifies a row through an ungated lookup and reads its owner
	// through a visibility-gated one, so a customer whose owning record the
	// reader may not open arrives here owner-less while somebody plainly owes
	// the reply. Saying `unassigned` would be a claim the workspace never made;
	// saying nothing at all would be indistinguishable from a lane that was
	// never wired, which is exactly what the census exists to catch.
	ownerUnreadable
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
	case ownerUnreadable:
		// Withheld, and the contract's own answer for a fact this caller may
		// not resolve: the field is absent. A client draws the row without an
		// owner rather than being told nobody owes it.
		return nil
	case ownerFromTheDeal:
		// The facts pass has run by now. THREE states, and conflating the last
		// two is what this spells out: the deal resolved and names an owner;
		// the deal resolved and names none, which is a real unassigned deal;
		// and the deal did not resolve at all, because the caller may not read
		// it — the same refusal shape the rest of the response uses, where the
		// honest answer is to say nothing rather than to report a withheld
		// owner as an absent one.
		//
		// `Deal` is the discriminator: applyDealFigures attaches it whenever
		// the figures came back, and leaves it nil when they did not.
		if row.item.Deal == nil {
			return nil
		}
		if row.item.Deal.OwnerId == nil {
			return &crmcontracts.WorklistOwner{Kind: crmcontracts.WorklistOwnerUnassigned}
		}
		return &crmcontracts.WorklistOwner{
			Kind: crmcontracts.WorklistOwnerUser,
			Id:   row.item.Deal.OwnerId,
		}
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
// An UNBOUND seam names nobody, which is the same absence.
//
// A FAILURE travels. The contract says an absent label means the caller may not
// resolve that name, so swallowing a database error here would publish a
// refusal the workspace never made — and a reader comparing two pages would see
// names appear and disappear with nothing to explain it. The row scope has
// already decided what this caller may see; a roster that will not answer is a
// fault, not a policy.
func (s *Service) nameTheOwners(ctx context.Context, queue []crmcontracts.WorklistItem) error {
	if s.teammates == nil || !anyOwnerNeedsAName(queue) {
		return nil
	}
	roster, _, err := s.teammates.LiveTeammatesOfCaller(ctx)
	if err != nil {
		return fmt.Errorf("attention: naming the owners on the queue: %w", err)
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
	return nil
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

// assigneeID is the assignee as the SCOPE filters read it.
//
// The same value ownerFromAssignee turns into the wire's answer, in the shape
// answersTo wants: a zero id where nobody holds the row. Both readings come off
// this one field so a task cannot be unowned to the filters and owned to the
// client — which is how an outside-team assignee reached both the page and the
// wire while `keepTeams` counted the row as nobody's.
func assigneeID(assignee *openapi_types.UUID) ids.UUID {
	if assignee == nil {
		return ids.UUID{}
	}
	return ids.UUID(*assignee)
}

// waitingOwner is the waiting lane's answer, where a zero id is ambiguous.
//
// Every other owner-column lane can read a zero as "nobody has taken it". This
// one cannot: its eligibility is qualified through an ungated lookup and its
// owner through a visibility-gated one, so a row whose owning record the reader
// may not open reaches here indistinguishable from an unowned customer.
//
// It says nothing in that case. A withheld owner reported as `unassigned` is a
// claim the workspace never made — somebody does owe that reply — and the
// reader has nothing on the row to tell them the difference.
func waitingOwner(owner ids.UUID) ownerRef {
	if owner.IsZero() {
		return ownerRef{kind: ownerUnreadable}
	}
	return ownedBy(owner)
}
