// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A door with a line of its own prefers it where the resolver's tells an
// approver less.
func TestReadsPoorlyJudgesWhatTheSentenceTellsAnApprover(t *testing.T) {
	target, other := ids.NewV7(), ids.NewV7()
	cases := map[string]struct {
		info StageInfo
		want bool
	}{
		"names the records":     {StageInfo{TargetID: target, Summary: `Archive "Acme"`}, false},
		"another id":            {StageInfo{TargetID: target, Summary: "x " + ids.NewV7().String()}, false},
		"no target at all":      {StageInfo{Summary: "Create a deal"}, false},
		"target by id":          {StageInfo{TargetID: target, Summary: "x " + target.String()}, true},
		"co-target by id":       {StageInfo{TargetID: target, CoTargetID: other, Summary: "x " + other.String()}, true},
		"fields without values": {StageInfo{Summary: "Update a deal, setting amount", FieldsOnly: true}, true},
		"no sentence":           {StageInfo{TargetID: target}, true},
	}
	for name, tc := range cases {
		if got := tc.info.ReadsPoorly(); got != tc.want {
			t.Errorf("%s: ReadsPoorly = %v, want %v", name, got, tc.want)
		}
	}
}
