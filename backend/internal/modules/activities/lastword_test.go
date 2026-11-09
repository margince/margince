// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
)

type tiedRow struct {
	name    string
	inbound bool
	day     bool
}

func readTied(at time.Time) func(tiedRow) (occurredday.Moment, bool) {
	return func(row tiedRow) (occurredday.Moment, bool) {
		return occurredday.Moment{At: at, DateOnly: row.day}, row.inbound
	}
}

// At one stored instant the record's move is read off their message, unless a
// day on either side makes ours its answer. The logging order never decides.
func TestLastWordReadsTheMoveByTheAnswerRuleNotTheId(t *testing.T) {
	at := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		name string
		tied []tiedRow
		want string
	}{
		{"their mail logged last, both instants", []tiedRow{{"theirs", true, false}, {"ours", false, false}}, "theirs"},
		{"our mail logged last, both instants", []tiedRow{{"ours", false, false}, {"theirs", true, false}}, "theirs"},
		{"their mail logged last, both days", []tiedRow{{"theirs", true, true}, {"ours", false, true}}, "ours"},
		{"our mail logged last, both days", []tiedRow{{"ours", false, true}, {"theirs", true, true}}, "ours"},
		{"their day beside our instant", []tiedRow{{"theirs", true, true}, {"ours", false, false}}, "ours"},
		{"two of ours", []tiedRow{{"newer", false, false}, {"older", false, false}}, "newer"},
		{"one of theirs alone", []tiedRow{{"theirs", true, true}}, "theirs"},
	} {
		if got := LastWord(c.tied, readTied(at)).name; got != c.want {
			t.Errorf("%s: the move is read off %q, want %q", c.name, got, c.want)
		}
	}
}
