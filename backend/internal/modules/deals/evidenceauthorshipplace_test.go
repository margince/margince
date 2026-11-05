// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestTheAuthorshipPlaceClearsAtTheTail(t *testing.T) {
	row := ids.NewV7()
	cases := []struct {
		name    string
		after   ids.UUID
		scanned int
		want    *ids.UUID
	}{
		{"a full page keeps its last row", row, evidenceAuthorshipBatch, &row},
		{"a short page is the tail", row, evidenceAuthorshipBatch - 1, nil},
		{"an empty page is the tail", ids.UUID{}, 0, nil},
		{"a full page with no last row keeps none", ids.UUID{}, evidenceAuthorshipBatch, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := authorshipPlaceAfter(c.after, c.scanned)
			switch {
			case c.want == nil && got != nil:
				t.Fatalf("the pass parks at %s, want it back at the first row", got)
			case c.want != nil && got == nil:
				t.Fatalf("the pass starts over, want it to carry on from %s", c.want)
			case c.want != nil && *got != *c.want:
				t.Fatalf("the pass carries on from %s, want %s", got, c.want)
			}
		})
	}
}
