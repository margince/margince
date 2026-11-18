// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package personname

import (
	"math"
	"testing"
)

// The spec pins Jaro: ("jon doe","john doe") = (7/7 + 7/8 + 7/7)/3.
func TestJaroReproducesTheSpecWorkedExample(t *testing.T) {
	if got := jaro("jon doe", "john doe"); math.Abs(got-0.9583) >= 0.0001 {
		t.Fatalf("jaro(jon doe, john doe) = %.4f, spec pins 0.9583", got)
	}
}

// Six matches with one transposed adjacent pair: (6/6 + 6/6 + 5/6)/3.
func TestJaroCountsTranspositions(t *testing.T) {
	if got := jaro("martha", "marhta"); math.Abs(got-0.9444) >= 0.0001 {
		t.Fatalf("jaro(martha, marhta) = %.4f, want 0.9444", got)
	}
}
