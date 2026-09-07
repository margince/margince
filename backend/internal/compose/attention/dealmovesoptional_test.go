// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// A suggested next step is an ENRICHMENT of a queue row, so a reader who may
// not read deals loses the suggestion and keeps the page.
//
// The two grants come apart. Role grants are edited per object, so a role
// holding activity:read without deal:read is one an administrator can write —
// and the activity lane admits a task on activity:read alone, so a task linked
// to a deal reaches that member's queue. Propagating the deal refusal from an
// enrichment would turn their legitimate worklist into a 403.
//
// A real fault still propagates. The distinction is the whole test: "you may
// not read deals" and "the database is down" must not produce the same page.

import (
	"context"
	"errors"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// refusingMoves answers the way dealstatus.Service answers a caller whose
// object grant it just refused.
type refusingMoves struct{ err error }

func (r refusingMoves) CachedMoves(context.Context, []ids.UUID) (map[ids.UUID]crmcontracts.DealStatusCardMove, error) {
	return nil, r.err
}

// answeringMoves is the positive control: a reader that works.
type answeringMoves struct {
	move crmcontracts.DealStatusCardMove
}

func (a answeringMoves) CachedMoves(_ context.Context, ids_ []ids.UUID) (map[ids.UUID]crmcontracts.DealStatusCardMove, error) {
	out := map[ids.UUID]crmcontracts.DealStatusCardMove{}
	for _, id := range ids_ {
		out[id] = a.move
	}
	return out, nil
}

func TestADealRefusalCostsTheStepAndNotTheQueue(t *testing.T) {
	deal := ids.NewV7()
	row := func() []crmcontracts.WorklistItem {
		return []crmcontracts.WorklistItem{{
			Subject: &crmcontracts.AttentionSubject{
				Type: subjectDeal, Id: openapi_types.UUID(deal),
			},
		}}
	}

	// The control first: the seam CAN put a step on this row, so the absence
	// below is the refusal and not a fixture that never reaches the reader.
	svc := (&Service{}).WithDealMoves(answeringMoves{
		move: crmcontracts.DealStatusCardMove{Action: "draft_email", Reason: "they are waiting on a reply"},
	})
	queue := row()
	if err := svc.nameTheStep(context.Background(), queue); err != nil {
		t.Fatalf("the answering reader failed the queue: %v", err)
	}
	if queue[0].Move == nil {
		t.Fatal("the answering reader put no step on the row, so this fixture cannot " +
			"tell a refusal from a seam that never fires")
	}

	// A refusal: the page survives, without the step.
	svc = (&Service{}).WithDealMoves(refusingMoves{err: apperrors.ErrPermissionDenied})
	queue = row()
	if err := svc.nameTheStep(context.Background(), queue); err != nil {
		t.Errorf("a member holding no deal grant lost the whole queue → %v — the step is "+
			"one column of a row they are entitled to see", err)
	}
	if queue[0].Move != nil {
		t.Errorf("a refused reader was given a step: %+v", queue[0].Move)
	}

	// A real fault is not absorbed.
	svc = (&Service{}).WithDealMoves(refusingMoves{err: errors.New("connection reset")})
	if err := svc.nameTheStep(context.Background(), row()); err == nil {
		t.Error("a database fault was swallowed — a queue whose rows have no step must not " +
			"be how a broken read looks")
	}
}
