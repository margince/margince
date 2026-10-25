// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package relstrength

import (
	"strings"
	"testing"
)

func TestSidesNameWhichWayItRuns(t *testing.T) {
	t.Parallel()
	cases := []struct {
		theyWrote, weWrote bool
		want               Direction
	}{
		{true, true, DirectionTwoWay},
		{true, false, DirectionInboundOnly},
		{false, true, DirectionOutboundOnly},
		{false, false, DirectionNone},
	}
	for _, c := range cases {
		if got := (Sides{TheyWrote: c.theyWrote, WeWrote: c.weWrote}).Direction(); got != c.want {
			t.Errorf("Sides{%v, %v}.Direction() = %q, want %q", c.theyWrote, c.weWrote, got, c.want)
		}
	}
}

func TestTheyWroteAsksWhoSentTheMessage(t *testing.T) {
	t.Parallel()
	rule := TheyWroteSQL("c.id", "a")
	for _, clause := range []string{"a.direction = 'inbound'", SenderSQL("c.id", "a")} {
		if !strings.Contains(rule, clause) {
			t.Errorf("TheyWroteSQL = %q lacks %q", rule, clause)
		}
	}
}
