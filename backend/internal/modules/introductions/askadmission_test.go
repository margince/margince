// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package introductions

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestAModelNoteAlwaysCarriesTheAIFlag(t *testing.T) {
	agent := principal.WithActor(context.Background(), principal.Principal{Type: principal.PrincipalAgent, PassportID: ids.NewV7(), OnBehalfOf: ids.NewV7()})
	human := principal.WithActor(context.Background(), principal.Principal{Type: principal.PrincipalHuman})
	for name, ctx := range map[string]context.Context{"agent": agent, "user": human} {
		got, err := admitAsk(ctx, NewRequest{InternalReason: "x", NoteGeneratedBy: "model", NoteAIGenerated: false})
		if err != nil {
			t.Fatal(err)
		}
		if !got.NoteAIGenerated {
			t.Errorf("%s: a note marked model came out with the AI flag off", name)
		}
	}
	typed, err := admitAsk(human, NewRequest{InternalReason: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if typed.NoteAIGenerated || typed.NoteGeneratedBy != "human" {
		t.Errorf("a note a user typed came out as %+v", typed)
	}
}
