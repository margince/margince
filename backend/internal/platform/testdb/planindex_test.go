// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package testdb

import "testing"

func TestIdentityLookupConditionExcludesOtherKeyColumns(t *testing.T) {
	for condition, want := range map[string]bool{
		"": false, "(domain_evidence_version = 1)": false, "(activity_id = a.id)": false,
		"(id = input.activity_id)": true, "(id = ANY ($1))": true, "((a.id = input.id) AND (domain_evidence_version = 1))": true,
	} {
		if got := IndexConditionReadsID(condition); got != want {
			t.Errorf("condition %q: got %v want %v", condition, got, want)
		}
	}
}
