// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"slices"
	"testing"
)

func TestRemovingAMaskKeepsAnIdenticalPopulationRestriction(t *testing.T) {
	original := []string{"FALSE", "t.archived_at IS NULL", "FALSE"}
	got := withoutMaskClauses(original, []string{"FALSE"})
	if !slices.Contains(got, "FALSE") || len(got) != 2 {
		t.Fatalf("population restriction was removed: %v", got)
	}
	if len(original) != 3 {
		t.Fatal("the compiled answer population was changed")
	}
}
