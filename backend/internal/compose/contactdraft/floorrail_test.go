// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contactdraft

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/compose/draftvoice"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// floorWitness is a lane that is down, and that records whether the call it
// refused was made under a declared floor.
type floorWitness struct{ declared []bool }

func (l *floorWitness) Complete(ctx context.Context, _ model.Request) (model.Response, error) {
	l.declared = append(l.declared, ai.ServesFloor(ctx))
	return model.Response{}, errors.New("the model is unavailable")
}

// A lane that fails still leaves the rep a draft, so the composer declares its
// floor on every call it makes: the rail then says the email was drafted with
// limited help, not that it failed while the draft sits in the composer.
func TestAFailedLaneIsAskedUnderTheComposersFloor(t *testing.T) {
	lane := &floorWitness{}
	_, by, err := Write(context.Background(), lane, floorInput(), draftvoice.Context{})
	if err != nil {
		t.Fatalf("a failed lane failed the draft: %v", err)
	}
	if by != crmcontracts.WrittenByDeterministic {
		t.Fatalf("generated_by = %v, want the floor", by)
	}
	if len(lane.declared) == 0 {
		t.Fatal("the lane was never asked, so this proves nothing about the floor")
	}
	for i, declared := range lane.declared {
		if !declared {
			t.Errorf("call %d was made with no declared floor, so the rail would say the draft failed", i+1)
		}
	}
}
