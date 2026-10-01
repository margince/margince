// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// What the morning digest may NAME, at the one edge a seeded row cannot reach.
//
// Everything else about this render is proven against a real database, because
// everything else is a question about rows. This is not: it is the arm that
// runs when the row scope has no answer for a target's type at all, and the
// product writes no notice pointing at such a record today — so a suite that
// tried to seed one would be seeding a row production never produces.

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/modules/notices"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A target type the row scope cannot answer about is COUNTED, never quoted.
//
// auth.VisibleSubset works by rendering a row-scope predicate, so it can only
// answer for a table the scope knows. Handed one it does not, it fails — and a
// failure there costs a colleague their whole morning rather than one line. So
// the filter sits BEFORE the probe, and such a target is simply absent from the
// answer, which readDigestLines reads as "count it and do not name it".
//
// NO DATABASE, and that is the assertion rather than a shortcut: an unscopable
// target must not reach a statement at all, so the nil transaction is what
// proves it. An edit that started probing these would panic here instead of
// erroring in somebody's inbox.
func TestAnUnscopableTargetIsNeverNamedInTheMorning(t *testing.T) {
	t.Parallel()

	unscopable := notices.Target{Type: "activity", ID: ids.NewV7()}
	// The case only stands for something while the scope really has no answer
	// for this type. If activities become row-scoped, this test has to move to
	// a type that is not, rather than quietly proving the ordinary path twice.
	if auth.RowScoped(unscopable.Type) {
		t.Fatalf("%q is row-scoped now, so this case no longer stands for a type VisibleSubset cannot answer about",
			unscopable.Type)
	}

	nameable, err := nameableTargets(context.Background(), nil, []notices.Notice{{
		ID:      ids.NewV7(),
		Kind:    notices.KindApprovalPending,
		Subject: "A proposal is waiting for your decision",
		Target:  unscopable,
	}})
	if err != nil {
		t.Fatalf("reading what the morning may name: %v", err)
	}
	if nameable[unscopable] {
		t.Error("a target the row scope cannot answer about came back nameable, so the message " +
			"would quote a record nothing had checked the reader may open")
	}
	if len(nameable) != 0 {
		t.Errorf("the answer names %d target(s), and no probe could have run without a transaction: %+v",
			len(nameable), nameable)
	}
}
