// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package httperr

import "testing"

func TestRequirePairRefusesExactlyTheHalfSetCases(t *testing.T) {
	for _, tc := range []struct {
		name          string
		first, second *string
		refused       bool
	}{
		{name: "neither"},
		{name: "both", first: new("contact"), second: new("x")},
		{name: "first only", first: new("contact"), refused: true},
		{name: "second only", second: new("x"), refused: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := RequirePair("entity_type", tc.first, "entity_id", tc.second)
			if (err != nil) != tc.refused {
				t.Fatalf("RequirePair = %v, want refused=%v", err, tc.refused)
			}
			if err == nil {
				return
			}
			if err.Status != 422 || len(err.Fields) != 1 || err.Fields[0].Field != "entity_type" || err.Fields[0].Code != "requires_pair" {
				t.Fatalf("refusal = %+v, want a 422 requires_pair on entity_type", err)
			}
		})
	}
}
