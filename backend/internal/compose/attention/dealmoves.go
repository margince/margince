// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// Worklist actions come from the same current rule evaluation as the deal page.

import (
	"context"
	"errors"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/owedwork"
)

// DealMoves checks the same deterministic rules as the record, over batched current facts.
// An absent deal has no verified action to offer; it cannot supply work on its own.
type DealMoves interface {
	CurrentMoves(ctx context.Context, dealIDs []ids.UUID) (map[ids.UUID]crmcontracts.DealStatusCardMove, error)
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
	var moves map[ids.UUID]crmcontracts.DealStatusCardMove
	var err error
	if s.currentDealMoves != nil {
		moves = s.currentDealMoves.moves
	} else {
		moves, err = s.dealMoves.CurrentMoves(ctx, wanted)
	}
	// A suggested move is an ENRICHMENT of a queue row: a reader who may not
	// read deals loses the suggestion and keeps the row, which is the answer
	// this function's own doc already gives for a move naming a record they
	// may no longer read. Propagating it would fail the whole page instead.
	if errors.Is(err, apperrors.ErrPermissionDenied) {
		return nil
	}
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
	if (item.Source != sourceBriefItem && item.Source != sourceAtRisk) ||
		item.Move != nil || item.Subject == nil || item.Subject.Type != subjectDeal {
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
// arms are exhaustive. unansweredInbound names the mail being answered, so its
// move carries an activity_id. firstOutreach names nobody yet, so its move
// carries no arguments at all. The follow-up after a meeting names the meeting
// and links the contacts it goes to. A `draft_email` with a record and no
// recipients is a reply. One with recipients starts a conversation, so it
// names no record to answer.
//
// Held by: TestAnUnansweredMailReachesTheRowAsAReply and
// TestAMeetingFollowUpReachesTheRowAsAFreshMessage (dealmoves_test.go)
func worklistMoveOf(decided crmcontracts.DealStatusCardMove) *crmcontracts.WorklistMove {
	out := &crmcontracts.WorklistMove{Action: crmcontracts.WorklistMoveAction(decided.Action), Basis: decided.Basis, MailCoverage: decided.MailCoverage, Recommendation: decided.Recommendation}
	if decided.Arguments == nil {
		return out
	}
	args := *decided.Arguments
	out.Arguments = &args
	if _, addressed := args["links"]; addressed && out.Action == crmcontracts.WorklistMoveActionDraftEmail {
		return out
	}
	if id, ok := NamedActivityArgument(args); ok {
		named := openapi_types.UUID(id)
		out.ActivityId = &named
		if out.Action == crmcontracts.WorklistMoveActionDraftEmail {
			out.Action = crmcontracts.WorklistMoveActionDraftReply
		}
	}
	return out
}

// NamedActivityArgument shares the current-source gate's parser, including typed operands.
func NamedActivityArgument(args map[string]any) (ids.UUID, bool) {
	return owedwork.ActivityArgument(args)
}
