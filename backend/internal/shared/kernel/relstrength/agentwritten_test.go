// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package relstrength

import "testing"

func TestRecordedByAgentIncludesEveryWritePath(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		origin, captured string
		want             bool
	}{
		{"agent", "human:member", true},
		{"human", "agent:passport", true},
		{"human", "agent:legacy-run", true},
		{"human", "human:member", false},
		{"human", "connector:mail", false},
		{"human", "human:agent:member", false},
		{"human", "", false},
	} {
		if got := RecordedByAgent(c.origin, c.captured); got != c.want {
			t.Errorf("origin=%q captured_by=%q: agent=%v, want %v", c.origin, c.captured, got, c.want)
		}
	}
}
