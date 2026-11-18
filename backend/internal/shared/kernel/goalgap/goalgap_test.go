// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package goalgap

import "testing"

func TestTheGapIsTheGoalLessTheLanding(t *testing.T) {
	t.Parallel()
	if got := Of(900, 600); got != 300 {
		t.Errorf("a landing 300 short reads a gap of %d", got)
	}
	if got := Of(600, 900); got != -300 {
		t.Errorf("a landing 300 over reads a gap of %d", got)
	}
}
