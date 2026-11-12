// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/pkg/extension"
)

// The read needs its declaration, answers only where no user gets the answer,
// and takes at most MaxResponsible contacts in a call.
func TestTheSetAsideReadNeedsItsDeclarationAndBoundsABatch(t *testing.T) {
	user, contact := ids.NewV7().String(), ids.NewV7().String()
	ctx := principal.WithWorkspaceID(context.Background(), ids.NewV7())
	binding := extensionRuntimeBinding{pool: &pgxpool.Pool{}}

	composeReader(t, extension.SystemReadDeals)
	undeclared := deliveryRuntimeFor(ctx, "dealmirror", "1.0.0", "subscription/mirror", binding)
	defer undeclared.release()
	if _, err := undeclared.SetAside().Contact(ctx, extension.UserID(user), contact); !errors.Is(err, extension.ErrUndeclaredRead) {
		t.Errorf("an undeclared Contact: err = %v, want ErrUndeclaredRead", err)
	}

	composeReader(t, extension.SystemReadSetAside)
	attended := runtimeFor(ctx, "dealmirror", "1.0.0", "tool/probe", binding)
	defer attended.release()
	if _, err := attended.SetAside().Contact(ctx, extension.UserID(user), contact); !errors.Is(err, extension.ErrAttendedRead) {
		t.Errorf("Contact on a signed-in call: err = %v, want ErrAttendedRead", err)
	}

	declared := deliveryRuntimeFor(ctx, "dealmirror", "1.0.0", "subscription/mirror", binding)
	defer declared.release()
	tooMany := make([]string, extension.MaxResponsible+1)
	for i := range tooMany {
		tooMany[i] = ids.NewV7().String()
	}
	if _, err := declared.SetAside().Contacts(ctx, extension.UserID(user), tooMany); !errors.Is(err, extension.ErrInvalid) {
		t.Errorf("a batch of %d: err = %v, want ErrInvalid", len(tooMany), err)
	}
	if _, err := declared.SetAside().Contact(ctx, extension.UserID(user), "not-an-id"); !errors.Is(err, extension.ErrInvalid) {
		t.Errorf("a malformed contact id: err = %v, want ErrInvalid", err)
	}
}
