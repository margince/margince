// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The receipts behind a score are shown in the order they were counted, and a
// row this reader cannot discover is left out rather than drawn blank.

func wireIDs(t *testing.T, held ...ids.UUID) []openapi_types.UUID {
	t.Helper()
	out := make([]openapi_types.UUID, 0, len(held))
	for _, id := range held {
		out = append(out, openapi_types.UUID(id))
	}
	return out
}

func TestTheWireIdsBecomeKernelIdsInOrder(t *testing.T) {
	t.Parallel()
	first, second, third := ids.NewV7(), ids.NewV7(), ids.NewV7()

	got := activityIDsOf(wireIDs(t, first, second, third))

	// Order is the claim. The ids arrive in the order they were counted — the
	// order a reader is asked to recognise — and a conversion that sorted or
	// deduped them would show receipts in a sequence matching nothing.
	want := []ids.UUID{first, second, third}
	if len(got) != len(want) {
		t.Fatalf("got %d ids, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("id %d is %s, want %s", i, got[i], want[i])
		}
	}
}

func TestAnEmptyIdListAsksForNothing(t *testing.T) {
	t.Parallel()
	if got := activityIDsOf(nil); len(got) != 0 {
		t.Errorf("an empty score produced %d ids to read", len(got))
	}
}
