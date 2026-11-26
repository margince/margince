// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// An absent target_id decodes to the zero id, which the self-merge guard cannot
// see. The refusal names the field before any lookup runs, so the caller is not
// told a deal it never named does not exist.
func TestMergeDealNamesAnAbsentTargetBeforeAnyLookup(t *testing.T) {
	store := &Store{}
	_, err := store.MergeDeal(context.Background(), ids.New[ids.DealKind](), ids.DealID{}, nil)
	fault, _ := httperr.Classify(err)
	if fault.Status != 422 || len(fault.Fields) != 1 || fault.Fields[0].Field != "target_id" {
		t.Fatalf("merge with no target_id → %+v, want a 422 naming target_id", fault)
	}
}
