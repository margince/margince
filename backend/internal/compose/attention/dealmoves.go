// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// The next-step pass: a row about a deal says what to do about it.
//
// The complaint the move field was built for is written into its own contract
// description — the product knew the buyer had written, knew nobody had
// answered, knew the answer was to draft a reply, and the page said only "no
// contact for 83 days". One lane fixed that: an unanswered message names the
// reply it is waiting for. Every other deal row still arrived naming a problem
// and no step.
//
// Nothing here decides a step. compose/dealstatus already decides one per deal,
// from records, deterministically, and caches it per reader; this reads what it
// wrote. Deciding again in a second place is how a queue row and the deal page
// standing on the same records end up naming different next moves.
//
// A CACHE MISS RENDERS NO MOVE. Assembling a card costs a timeline, seats, a
// deal room and possibly a model call, and a page holds thirty rows. So a deal
// nobody has opened contributes nothing and its row says what it said before —
// the rule dealfacts.go and labels.go state, for the same reason.
//
// Read AFTER the page is cut, unlike the figures pass. Figures feed the
// ordering and so must be in hand before anything ranks; a move is drawn and
// never ranked, so reading one for a row the caller will not receive would
// spend a query on nothing.

import (
	"context"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// DealMoves answers the already-decided next step for each of these deals.
//
// Its implementation reads a cache and never assembles, which is what makes it
// safe to call with a whole page of ids. A deal absent from the answer has no
// step to suggest: no cached card, or a card whose own answer was that there is
// nothing to do.
type DealMoves interface {
	CachedMoves(ctx context.Context, dealIDs []ids.UUID) (map[ids.UUID]crmcontracts.DealStatusCardMove, error)
}

// nameTheStep puts each deal row's already-decided next step onto it.
//
// A row that already carries a move keeps it. classifyWaiting names the message
// a reply would answer, from the row's own id, and that is the more specific
// answer: it knows WHICH mail is waiting, where the card reasons about the deal
// as a whole.
func (s *Service) nameTheStep(ctx context.Context, queue []crmcontracts.WorklistItem) error {
	if s.dealMoves == nil {
		return nil
	}
	wanted := make([]ids.UUID, 0, len(queue))
	seen := map[ids.UUID]bool{}
	for i := range queue {
		id, ok := needsDealMove(queue[i])
		if !ok || seen[id] {
			continue
		}
		seen[id] = true
		wanted = append(wanted, id)
	}
	if len(wanted) == 0 {
		return nil
	}
	moves, err := s.dealMoves.CachedMoves(ctx, wanted)
	if err != nil {
		return err
	}
	for i := range queue {
		id, ok := needsDealMove(queue[i])
		if !ok {
			continue
		}
		decided, found := moves[id]
		if !found {
			continue
		}
		queue[i].Move = worklistMoveOf(decided)
	}
	return nil
}

// needsDealMove answers which deal a row is about, where the row is about a
// deal and carries no step of its own.
func needsDealMove(item crmcontracts.WorklistItem) (ids.UUID, bool) {
	if item.Move != nil || item.Subject == nil || item.Subject.Type != subjectDeal {
		return ids.UUID{}, false
	}
	return ids.UUID(item.Subject.Id), true
}

// worklistMoveOf carries one decided step onto the wire.
//
// Every verb travels unchanged EXCEPT one, and that exception is the whole of
// this function's judgement. The card spells both mail moves `draft_email` —
// writing to somebody for the first time, and answering a mail nobody replied
// to — because its own surface draws neither as a button and the distinction
// never had to reach a label. This queue does draw them, and it draws them
// differently: `draft_reply` opens the thread, `draft_email` starts one, and
// the contract says in as many words that collapsing the two answers a waiting
// buyer with a fresh message and no thread behind it.
//
// The OPERAND is what tells them apart, and it is not a heuristic. The card's
// arms are exhaustive: unansweredInbound names the mail being answered, so its
// move carries an activity_id; firstOutreach names nobody yet, so its move
// carries no arguments at all. A `draft_email` with a record is a reply.
//
// Held by: TestAnUnansweredMailReachesTheRowAsAReply (dealmoves_test.go)
func worklistMoveOf(decided crmcontracts.DealStatusCardMove) *crmcontracts.WorklistMove {
	out := &crmcontracts.WorklistMove{Action: crmcontracts.WorklistMoveAction(decided.Action)}
	if decided.Arguments == nil {
		return out
	}
	args := *decided.Arguments
	if id, ok := NamedActivityArgument(args); ok {
		named := openapi_types.UUID(id)
		out.ActivityId = &named
		if out.Action == crmcontracts.WorklistMoveActionDraftEmail {
			out.Action = crmcontracts.WorklistMoveActionDraftReply
		}
	}
	out.Arguments = &args
	return out
}

// NamedActivityArgument reads the record a verb acts on out of the card's arguments.
//
// Nothing rather than a zero id for a value this cannot parse. A move naming no
// record is still drawable for the verbs that need none, where one naming a
// record that does not exist is a control that fails when it is pressed.
//
// This is the SECOND reader of that key — the moves reader parses it too, to
// decide which records to re-gate against the caller's audience. The two are
// deliberately not shared: they sit in different compose packages, and a
// package here importing that one would be the sibling edge the seam interface
// exists to avoid. What holds them together instead is that a DISAGREEMENT is
// safe in one direction only, and it is this one: an id this parses but the
// gate did not would be an id served ungated. So this parse must be no more
// permissive than the gate's, which is why both are the same three refusals
// (absent key, non-string value, unparseable uuid) written the same way.
//
// EXPORTED for that gate alone. It is a pure function of a contract type, so
// exporting costs nothing, and the alternative — a test-only export, or a gate
// that reads the source text instead of running the code — would either hide
// the coupling or assert on spelling rather than behaviour.
//
// Held by: TestBothSidesReadTheRecordArgumentAlike (backend/gates)
func NamedActivityArgument(args map[string]any) (ids.UUID, bool) {
	raw, present := args["activity_id"]
	if !present {
		return ids.UUID{}, false
	}
	text, ok := raw.(string)
	if !ok {
		return ids.UUID{}, false
	}
	id, err := ids.Parse(text)
	if err != nil {
		return ids.UUID{}, false
	}
	return id, true
}
