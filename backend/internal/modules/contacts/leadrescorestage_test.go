// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import "testing"

// A column the patch leaves alone keeps the row's value. One it touches reads as
// the value it will write, nil for a clear.
func TestStagedColumnsReadWhatThePatchWillLeaveInTheRow(t *testing.T) {
	after := map[string]any{
		"source":   "referral",
		"title":    "CTO",
		"score":    42,
		"gone":     nil,
		"computed": 7,
	}
	current := "manual"
	if got := stagedString(after, "source", current); got != "referral" {
		t.Errorf("staged source = %q, want referral", got)
	}
	if got := stagedString(after, "status", "new"); got != "new" {
		t.Errorf("unstaged status = %q, want the current new", got)
	}
	if got := stagedOptionalString(after, "title", &current); got == nil || *got != "CTO" {
		t.Errorf("staged title = %v, want CTO", got)
	}
	if got := stagedOptionalString(after, "company", &current); got != &current {
		t.Errorf("unstaged company should be the current pointer, got %v", got)
	}
	if got := stagedOptionalString(after, "gone", &current); got != nil {
		t.Errorf("a cleared column should read nil, got %q", *got)
	}
	if got := stagedInt(after, "score", 1); got != 42 {
		t.Errorf("staged score = %d, want 42", got)
	}
	if got := stagedInt(after, "other", 1); got != 1 {
		t.Errorf("unstaged score = %d, want the current 1", got)
	}
}

func TestAStagedMachineScoreIsHeldOnlyWhileItIsAValue(t *testing.T) {
	five := 5
	after := map[string]any{"computed": 7, "cleared": nil}
	if n, held := stagedOptionalInt(after, "computed", &five); !held || n != 7 {
		t.Errorf("staged computed = %d, %v; want 7, true", n, held)
	}
	if _, held := stagedOptionalInt(after, "cleared", &five); held {
		t.Error("a column the patch sets to NULL holds no value")
	}
	if n, held := stagedOptionalInt(after, "unstaged", &five); !held || n != 5 {
		t.Errorf("unstaged computed = %d, %v; want the current 5, true", n, held)
	}
	if _, held := stagedOptionalInt(after, "unstaged", nil); held {
		t.Error("a NULL row value holds no value")
	}
}
