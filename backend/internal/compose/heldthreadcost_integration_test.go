// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// What a populated held queue costs the Connections surfaces that read it.

import (
	"fmt"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A held queue grows during a classifier outage. That is when the Connections
// surfaces matter most, so the read must not spend a statement per thread.
//
// This pins the statement count, not the payload, which does grow.
func TestTheHeldThreadReadCostsTheSameAsTheQueueGrows(t *testing.T) {
	e := integration.Setup(t)

	seedHeldQueue(t, e, e.Rep1, "small", 2)
	small, smallRows := heldThreadReadCost(t, e, e.Rep1)
	if smallRows != 2 {
		t.Fatalf("%d threads held, want the 2 seeded", smallRows)
	}

	seedHeldQueue(t, e, e.Rep1, "large", 18)
	large, largeRows := heldThreadReadCost(t, e, e.Rep1)
	if largeRows != 20 {
		t.Fatalf("%d threads held, want all 20 seeded", largeRows)
	}

	if large != small {
		t.Errorf("reading %d threads took %d statements and reading %d took %d: the read grows with the queue",
			largeRows, large, smallRows, small)
	}
	t.Logf("held threads %d → %d statements, %d → %d", smallRows, small, largeRows, large)
}

// seedHeldQueue holds n threads for one seat, each under its own key.
func seedHeldQueue(t *testing.T, e *integration.Env, user ids.UUID, label string, n int) {
	t.Helper()
	for i := range n {
		seedVerdict(t, e, user, fmt.Sprintf("thread-%s-%d", label, i), "pending", "", 1)
	}
}

// heldThreadReadCost is the statements one read spends, and the rows it found.
func heldThreadReadCost(t *testing.T, e *integration.Env, user ids.UUID) (int64, int) {
	t.Helper()
	metered, meter := database.WithStatementMeter(purgeCtx(e, user))
	got, err := capture.HeldThreadsFor(metered, InstallationDB(e.Pool))
	if err != nil {
		t.Fatalf("listing held threads: %v", err)
	}
	return meter.Cost().Statements, len(got)
}
