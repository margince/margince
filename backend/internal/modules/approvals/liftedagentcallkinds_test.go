// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

import (
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"strings"
	"testing"
)

// The withdrawal task shipped with these kinds. A tool that waits for its user
// since, such as a relink to a project, does not change what the task closes.
func TestTheLiftedAgentCallKindsAreTheShippedSet(t *testing.T) {
	kinds := LiftedAgentCallKinds()
	if !slices.IsSorted(kinds) || len(slices.Compact(slices.Clone(kinds))) != len(kinds) {
		t.Fatal("the frozen kinds must stay sorted and unique")
	}
	sum := sha256.Sum256([]byte(strings.Join(kinds, "\n")))
	if got := hex.EncodeToString(sum[:]); len(kinds) != 716 || got != frozenKindsDigest {
		t.Errorf("the shipped kind set changed: %d kinds, digest %s; a shipped data task must withdraw the same kinds on every upgrade", len(kinds), got)
	}
	for _, kind := range []string{"relink_activity", "relink_activities"} {
		if !slices.Contains(kinds, kind) {
			t.Errorf("%s ran at once for its user when the task shipped, so the task closes its approvals", kind)
		}
	}
	for _, kind := range []string{"enrich", "merge_member_accounts", "set_up_partner", "relink_thread"} {
		if slices.Contains(kinds, kind) {
			t.Errorf("%s waited for its user when the task shipped, so the task leaves its approvals", kind)
		}
	}
}

const frozenKindsDigest = "d3cde370877a6a82c54533b24c6628c1fef1843115c3f864c5b71ef479b78235"
