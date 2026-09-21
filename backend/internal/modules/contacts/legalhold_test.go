// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// The hold writer's fail-closed arm.

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A table name this module does not own is refused, and refused before the
// authority check can admit it.
//
// The exported HeldTable constants are the only way to reach this method
// legitimately, so this arm is not reachable through them — which is the
// argument for testing it rather than asserting it in a comment. A statement
// built from an unrecognised name is what this exists to prevent, and only a
// test notices if the switch stops refusing.
func TestTheHoldWriterRefusesATableThisModuleDoesNotOwn(t *testing.T) {
	t.Parallel()
	ctx := principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + ids.NewV7().String(),
	})
	// A nil store on purpose: the refusal must land before any transaction is
	// opened, so a version that reached the database would panic rather than
	// return a tidy error.
	err := (&Store{}).SetLegalHold(ctx, HeldTable("invoice"), ids.NewV7(), true, "Anwaltsschreiben")
	if err == nil {
		t.Fatal("a hold was accepted against a table this module does not own")
	}
	if errors.Is(err, apperrors.ErrInvalidArgument) || errors.Is(err, apperrors.ErrPermissionDenied) {
		return
	}
	t.Errorf("refused with %v, want ErrInvalidArgument or ErrPermissionDenied", err)
}
