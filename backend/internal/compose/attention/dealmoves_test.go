// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// The next-step pass.
//
// The row it exists for is a deal drifting: before this it named how long the
// deal had been quiet and nothing a rep could do about it, while the deal's own
// page — reading the same records — said which mail to answer.

import (
	"context"
	"errors"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type stubDealMoves struct {
	moves map[ids.UUID]crmcontracts.DealStatusCardMove
	asked []ids.UUID
	calls int
	err   error
}

func (s *stubDealMoves) CachedMoves(
	_ context.Context, dealIDs []ids.UUID,
) (map[ids.UUID]crmcontracts.DealStatusCardMove, error) {
	s.calls++
	s.asked = append(s.asked, dealIDs...)
	return s.moves, s.err
}

func riskRow(dealID ids.UUID) crmcontracts.WorklistItem {
	return crmcontracts.WorklistItem{
		Id:       dealID.String(),
		Source:   "deal_at_risk",
		Category: "deals_at_risk",
		Actions:  []crmcontracts.WorklistItemActions{},
		Subject: &crmcontracts.AttentionSubject{
			Type: subjectDeal,
			Id:   openapi_types.UUID(dealID),
		},
	}
}

func cardMove(action string, args map[string]any) crmcontracts.DealStatusCardMove {
	out := crmcontracts.DealStatusCardMove{Action: action}
	if args != nil {
		out.Arguments = &args
	}
	return out
}

// THE case this pass exists for. A drifting deal arrives naming a problem; the
// card already decided what to do about it, and the row carries that verb and
// that record rather than sending the reader to a second page to find them.
//
// The meeting verb, deliberately: it is the one that travels UNCHANGED, so this
// asserts the ordinary pass-through rather than the single translation
// worklistMoveOf makes (which has its own test below).
func TestADriftingDealCarriesTheCardsMove(t *testing.T) {
	dealID := ids.NewV7()
	booked := ids.NewV7()
	svc := (&Service{}).WithDealMoves(&stubDealMoves{
		moves: map[ids.UUID]crmcontracts.DealStatusCardMove{
			dealID: cardMove("open_meeting_brief", map[string]any{"activity_id": booked.String()}),
		},
	})
	queue := []crmcontracts.WorklistItem{riskRow(dealID)}

	if err := svc.nameTheStep(context.Background(), queue); err != nil {
		t.Fatalf("naming the step: %v", err)
	}

	move := queue[0].Move
	if move == nil {
		t.Fatal("the drifting deal still names no step — the row the whole pass exists for")
	}
	if move.Action != crmcontracts.WorklistMoveActionOpenMeetingBrief {
		t.Fatalf("the row suggests %q, wanted the card's own verb open_meeting_brief", move.Action)
	}
	// The RECORD, not just the verb. "Read the brief" with nothing named is a
	// control the client cannot draw, which is the state this replaces.
	if move.ActivityId == nil || ids.UUID(*move.ActivityId) != booked {
		t.Fatalf("the move names %v, wanted the meeting the card picked %s", move.ActivityId, booked)
	}
}

// A deal nobody has opened has no cached card, and assembling one costs a
// timeline, seats and possibly a model call. Thirty rows would be thirty of
// those, which is the N+1 this feed exists to avoid — so the row keeps what it
// had and says no step.
func TestADealWithNoCachedCardKeepsItsRowUnchanged(t *testing.T) {
	dealID := ids.NewV7()
	moves := &stubDealMoves{moves: map[ids.UUID]crmcontracts.DealStatusCardMove{}}
	svc := (&Service{}).WithDealMoves(moves)
	queue := []crmcontracts.WorklistItem{riskRow(dealID)}

	if err := svc.nameTheStep(context.Background(), queue); err != nil {
		t.Fatalf("naming the step: %v", err)
	}

	if queue[0].Move != nil {
		t.Fatalf("a deal with no cached card invented the step %+v", queue[0].Move)
	}
	if moves.calls != 1 {
		t.Fatalf("the pass made %d reads for one page, wanted exactly one", moves.calls)
	}
}

// One read for the whole page, and each deal asked for once. A row-at-a-time
// read is the shape this pass was written to avoid, and a duplicate id in the
// ask is that shape half-arrived.
func TestThePageIsReadOnceAndEachDealAskedForOnce(t *testing.T) {
	first, second := ids.NewV7(), ids.NewV7()
	moves := &stubDealMoves{moves: map[ids.UUID]crmcontracts.DealStatusCardMove{}}
	svc := (&Service{}).WithDealMoves(moves)
	// The same deal twice: a queue can hold two rows about one deal, and asking
	// for it twice would be a page-sized read that still grows per row.
	queue := []crmcontracts.WorklistItem{riskRow(first), riskRow(second), riskRow(first)}

	if err := svc.nameTheStep(context.Background(), queue); err != nil {
		t.Fatalf("naming the step: %v", err)
	}

	if moves.calls != 1 {
		t.Fatalf("the pass made %d reads for one page, wanted exactly one", moves.calls)
	}
	// WHICH deals, not merely how many. A cardinality check alone passes when the
	// pass asks for `first` twice and never asks about `second` at all — the same
	// count, one deal silently unenriched.
	asked := map[ids.UUID]int{}
	for _, id := range moves.asked {
		asked[id]++
	}
	if asked[first] != 1 || asked[second] != 1 || len(asked) != 2 {
		t.Fatalf("the pass asked for %v over three rows naming two deals, "+
			"wanted each of %s and %s exactly once", asked, first, second)
	}
}

// A waiting row names the message a reply would answer, from its own id. The
// card reasons about the deal as a whole and knows no such message, so the more
// specific answer stands rather than being overwritten by the general one.
func TestARowThatAlreadyNamesItsStepKeepsIt(t *testing.T) {
	dealID := ids.NewV7()
	waiting := ids.NewV7()
	svc := (&Service{}).WithDealMoves(&stubDealMoves{
		moves: map[ids.UUID]crmcontracts.DealStatusCardMove{
			dealID: cardMove("create_task", map[string]any{"subject": "Agree the next step"}),
		},
	})
	row := riskRow(dealID)
	answers := openapi_types.UUID(waiting)
	row.Move = &crmcontracts.WorklistMove{
		Action:     crmcontracts.WorklistMoveActionDraftReply,
		ActivityId: &answers,
	}
	queue := []crmcontracts.WorklistItem{row}

	if err := svc.nameTheStep(context.Background(), queue); err != nil {
		t.Fatalf("naming the step: %v", err)
	}

	if queue[0].Move.Action != crmcontracts.WorklistMoveActionDraftReply {
		t.Fatalf("the card's general step overwrote the row's own: %q", queue[0].Move.Action)
	}
	if queue[0].Move.ActivityId == nil || ids.UUID(*queue[0].Move.ActivityId) != waiting {
		t.Fatalf("the row lost the message it was waiting on: %v", queue[0].Move.ActivityId)
	}
}

// A verb whose operand is not a record carries its arguments and names none.
// Requiring a record here would make create_task undrawable, which is why the
// contract relaxed activity_id in the first place.
func TestAStepWithNoRecordStillTravels(t *testing.T) {
	dealID := ids.NewV7()
	svc := (&Service{}).WithDealMoves(&stubDealMoves{
		moves: map[ids.UUID]crmcontracts.DealStatusCardMove{
			dealID: cardMove("create_task", map[string]any{
				"subject": "Agree the next step on Acme",
				"source":  "ui",
			}),
		},
	})
	queue := []crmcontracts.WorklistItem{riskRow(dealID)}

	if err := svc.nameTheStep(context.Background(), queue); err != nil {
		t.Fatalf("naming the step: %v", err)
	}

	move := queue[0].Move
	if move == nil || move.Action != crmcontracts.WorklistMoveActionCreateTask {
		t.Fatalf("the create_task step did not reach the row: %+v", move)
	}
	if move.ActivityId != nil {
		t.Fatalf("a step acting on no record named one: %v", move.ActivityId)
	}
	if move.Arguments == nil {
		t.Fatal("the step reached the row with no arguments — the client has nothing to send")
	}
	if (*move.Arguments)["subject"] != "Agree the next step on Acme" {
		t.Fatalf("the verb's own operand did not travel: %+v", *move.Arguments)
	}
}

// The one verb this pass translates, and why it must.
//
// The card spells BOTH mail moves `draft_email` — its own surface draws neither
// as a button, so the difference never had to reach a label. This queue draws
// them, and differently: a reply opens the thread the buyer is waiting on, a
// fresh message starts a new one. A row reading "Draft the email" over an
// unanswered mail would answer that buyer outside their own thread, which is
// the failure the contract names when it refuses to collapse the two verbs.
//
// The card's own reason for this move says "draft the reply" in as many words,
// so the row saying anything else is the two halves of one screen disagreeing.
func TestAnUnansweredMailReachesTheRowAsAReply(t *testing.T) {
	dealID := ids.NewV7()
	waiting := ids.NewV7()
	svc := (&Service{}).WithDealMoves(&stubDealMoves{
		moves: map[ids.UUID]crmcontracts.DealStatusCardMove{
			// Exactly what dealstatus/move.go's unansweredInbound arm builds.
			dealID: cardMove("draft_email", map[string]any{"activity_id": waiting.String()}),
		},
	})
	queue := []crmcontracts.WorklistItem{riskRow(dealID)}

	if err := svc.nameTheStep(context.Background(), queue); err != nil {
		t.Fatalf("naming the step: %v", err)
	}

	move := queue[0].Move
	if move.Action != crmcontracts.WorklistMoveActionDraftReply {
		t.Fatalf("an unanswered mail reached the row as %q — the reader is offered a fresh message "+
			"where the card said to answer the one they are waiting on", move.Action)
	}
	if move.ActivityId == nil || ids.UUID(*move.ActivityId) != waiting {
		t.Fatalf("the reply names %v, wanted the mail being answered %s", move.ActivityId, waiting)
	}
}

// A deal nobody has contacted yet. The card's firstOutreach arm builds its move
// with NO arguments — move(ActionDraftEmail, reason, nil) — and the card
// normalizes that nil to an empty object rather than leaving it null. So this is
// a shape production really produces, and the row must still offer the verb: a
// first outreach names no message because there is no message yet.
func TestAnOpeningOutreachCarriesNoRecordAndStillDraws(t *testing.T) {
	dealID := ids.NewV7()
	svc := (&Service{}).WithDealMoves(&stubDealMoves{
		moves: map[ids.UUID]crmcontracts.DealStatusCardMove{
			dealID: cardMove("draft_email", map[string]any{}),
		},
	})
	queue := []crmcontracts.WorklistItem{riskRow(dealID)}

	if err := svc.nameTheStep(context.Background(), queue); err != nil {
		t.Fatalf("naming the step: %v", err)
	}

	move := queue[0].Move
	if move == nil || move.Action != crmcontracts.WorklistMoveActionDraftEmail {
		t.Fatalf("the opening outreach did not reach the row: %+v", move)
	}
	if move.ActivityId != nil {
		t.Fatalf("a move built with no arguments named a record: %v", move.ActivityId)
	}
}

// An activity_id the card wrote as something other than a uuid names no record
// this client can open. Carrying it through would draw a control that fails
// when it is pressed, which is worse than drawing none.
func TestAnUnreadableRecordArgumentNamesNoRecord(t *testing.T) {
	dealID := ids.NewV7()
	svc := (&Service{}).WithDealMoves(&stubDealMoves{
		moves: map[ids.UUID]crmcontracts.DealStatusCardMove{
			dealID: cardMove("draft_email", map[string]any{"activity_id": "not-a-uuid"}),
		},
	})
	queue := []crmcontracts.WorklistItem{riskRow(dealID)}

	if err := svc.nameTheStep(context.Background(), queue); err != nil {
		t.Fatalf("naming the step: %v", err)
	}

	if queue[0].Move.ActivityId != nil {
		t.Fatalf("an unparseable record reached the wire as %v", queue[0].Move.ActivityId)
	}
}

// A row about a person is not a deal row. Asking for it would send a person's
// id into a deal-keyed read, and the answer would be a miss dressed as one.
func TestOnlyDealRowsAreAskedAbout(t *testing.T) {
	moves := &stubDealMoves{moves: map[ids.UUID]crmcontracts.DealStatusCardMove{}}
	svc := (&Service{}).WithDealMoves(moves)
	person := riskRow(ids.NewV7())
	person.Subject.Type = "person"
	queue := []crmcontracts.WorklistItem{person}

	if err := svc.nameTheStep(context.Background(), queue); err != nil {
		t.Fatalf("naming the step: %v", err)
	}

	if moves.calls != 0 {
		t.Fatalf("a person row provoked %d deal reads, wanted none", moves.calls)
	}
}

// An installation with no card service is one whose deal rows say what they
// said before this pass existed. Absent is not empty and it is not an error.
func TestAnUnboundMoveReaderLeavesEveryRowAlone(t *testing.T) {
	queue := []crmcontracts.WorklistItem{riskRow(ids.NewV7())}

	if err := (&Service{}).nameTheStep(context.Background(), queue); err != nil {
		t.Fatalf("an unbound reader failed the page: %v", err)
	}

	if queue[0].Move != nil {
		t.Fatalf("an unbound reader invented the step %+v", queue[0].Move)
	}
}

// A refused read is named, never folded into "this deal has no step". The two
// look identical on the row, and a page that cannot say which is which would
// report a quiet deal where it means it could not ask.
func TestARefusedReadFailsThePage(t *testing.T) {
	refused := errors.New("the card cache refused this reader")
	svc := (&Service{}).WithDealMoves(&stubDealMoves{err: refused})
	queue := []crmcontracts.WorklistItem{riskRow(ids.NewV7())}

	err := svc.nameTheStep(context.Background(), queue)

	if !errors.Is(err, refused) {
		t.Fatalf("a refused read answered %v, wanted the refusal to travel", err)
	}
}
