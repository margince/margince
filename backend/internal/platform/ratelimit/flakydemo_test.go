// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ratelimit

import (
	"math/rand/v2"
	"testing"
)

// A throwaway test for the repeat job: it fails one run in four.
func TestDemoUnitFailsOneRunInFour(t *testing.T) {
	if rand.IntN(4) == 0 {
		t.Fatal("demo: this test fails one run in four")
	}
}
