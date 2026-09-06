// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// The recency number and the record that explains it come from ONE snapshot.
//
// The composite runs at READ COMMITTED, where every statement takes its own
// snapshot. Read apart, `deal.last_activity_at` and the freshest activity on
// the deal are two answers about two moments: an activity committed between
// them is counted by the later statement and not by the earlier, so the card
// cites a message as the reason for a staleness that message disproves. A
// reader can see the contradiction and nothing in the product can explain it.
//
// Held STRUCTURALLY rather than by a race, and that is the honest instrument
// here: once the two values come from one statement the defect is impossible by
// construction, so there is no interleaving left for a behavioural test to
// arrange — a sequential one passes just as well against the two-statement
// version it is meant to refuse.
//
// What this reads is the SQL the file sends, through the same reader every
// census uses, so a statement assembled with `+` is judged whole rather than in
// fragments.

import (
	"os"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

func TestTheRecencyNumberAndItsEvidenceAreReadTogether(t *testing.T) {
	t.Parallel()

	const source = "health.go"
	text, err := os.ReadFile(source)
	if err != nil {
		t.Fatalf("reading %s: %v", source, err)
	}

	var carriesTheTimestamp []string
	for _, statement := range gatekit.SQLStatementsIn(t, source, string(text)) {
		if strings.Contains(statement, "last_activity_at") {
			carriesTheTimestamp = append(carriesTheTimestamp, statement)
		}
	}
	if len(carriesTheTimestamp) == 0 {
		t.Fatal("no statement in health.go reads last_activity_at — the composite no longer " +
			"reads the timestamp this test is about, so it is checking nothing")
	}
	for _, statement := range carriesTheTimestamp {
		// The evidence is the freshest activity ON THE DEAL, which is what the
		// link join says. A statement reading the timestamp without it is
		// reading half the answer and leaving the other half to a second
		// snapshot.
		if !strings.Contains(statement, "activity_link") {
			t.Errorf("a statement reads last_activity_at without the activity it names:\n%s\n\n"+
				"\tRead them together. Apart, they are two snapshots under READ COMMITTED, and an "+
				"activity committed between them makes the evidence contradict the number it explains.",
				strings.TrimSpace(statement))
		}
	}
}
