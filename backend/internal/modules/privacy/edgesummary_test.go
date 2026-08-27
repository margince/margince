// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestEdgeSummaryNamesTheOtherEndAndWhatMoved(t *testing.T) {
	acme := "Acme Corp"
	edge := edgeSubject{kind: "employment", otherID: strayEdgeID, otherLabel: &acme}

	for _, tc := range []struct {
		name    string
		action  string
		edge    edgeSubject
		after   map[string]any
		want    string
		phrased bool
	}{
		{
			name: "a link names the role it was made with", action: "create", edge: edge,
			after: map[string]any{"kind": "employment", "role": "cto"},
			want:  "Uma linked Acme Corp as cto", phrased: true,
		},
		{
			// A partner edge carries no role, and the kind is still information: it
			// says which of the seven kinds of link this was.
			name: "a roleless link names its kind instead", action: "create",
			edge:  edgeSubject{kind: "co_sell_with", otherID: strayEdgeID, otherLabel: &acme},
			after: map[string]any{"kind": "co_sell_with", "role": nil},
			want:  "Uma linked Acme Corp as co_sell_with", phrased: true,
		},
		{
			name: "a patch names the fields that moved, sorted", action: "update", edge: edge,
			after: map[string]any{"role": "coo", "started_at": "2026-01-01", "kind": "employment"},
			want:  "Uma changed Acme Corp's role, started_at", phrased: true,
		},
		{
			// The write narrows a patch's image to what changed, so an empty one is
			// a patch that moved nothing this image reports. The line says that much
			// and no more.
			name:   "a patch whose image reports nothing still reads as a change",
			action: "update", edge: edge, after: map[string]any{"kind": "employment"},
			want: "Uma changed Acme Corp's link", phrased: true,
		},
		{
			name: "an unlink names only the record it let go", action: actionArchive, edge: edge,
			after: map[string]any{"kind": "employment", "role": "cto"},
			want:  "Uma unlinked Acme Corp", phrased: true,
		},
		{
			// A verb an edge write does not emit has no edge phrasing, and inventing
			// one would claim to know what happened.
			name: "an unknown verb has no edge phrasing", action: "merge", edge: edge,
			after: nil, want: "", phrased: false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, phrased := edgeSummary("Uma", tc.action, tc.edge, tc.after)
			if phrased != tc.phrased || got != tc.want {
				t.Errorf("edgeSummary(%q) = %q (phrased=%v), want %q (phrased=%v)",
					tc.action, got, phrased, tc.want, tc.phrased)
			}
		})
	}
}

func TestEdgeSummaryFallsBackToTheOtherEndsIDRatherThanABlank(t *testing.T) {
	// A record whose name column is empty is still a record a reader can open, so
	// the line names its id — never an invented name, and never "linked  as cto".
	for _, label := range []*string{nil, strPtr("")} {
		got, phrased := edgeSummary("Uma", "create",
			edgeSubject{kind: "employment", otherID: strayEdgeID, otherLabel: label},
			map[string]any{"role": "cto"})
		want := "Uma linked " + strayEdgeID.String() + " as cto"
		if !phrased || got != want {
			t.Errorf("with label %v: %q, want %q", label, got, want)
		}
	}
}

func TestTheDelegatedSubjectReadsTheSameOnAnEdgeLine(t *testing.T) {
	// Attribution that read one way on a record line and another on an edge line,
	// on the same page, is what this phrasing exists to prevent: a person can be
	// asked about a change, and a machine is not a party to anything.
	devin := "Devin"
	subject := recordSummarySubject(actorTypeAgent, "agent:enrich", &devin, false, nil)
	edgeLine, phrased := edgeSummary(subject, actionArchive,
		edgeSubject{kind: "employment", otherID: strayEdgeID, otherLabel: strPtr("Acme Corp")}, nil)
	if !phrased || edgeLine != "Devin, via an agent, unlinked Acme Corp" {
		t.Errorf("edge line = %q", edgeLine)
	}
	recordLine := composeRecordSummary(actorTypeAgent, "agent:enrich", &devin, actionArchive, false, nil)
	if recordLine != "Devin, via an agent, archived the record" {
		t.Errorf("record line = %q — the two lines no longer share a subject", recordLine)
	}
}

// strayEdgeID stands for the other end's id in the cases where the label is what
// is under test. Fixed rather than random so a failure message is comparable
// between runs.
var strayEdgeID = ids.MustParse("01a03e00-0000-7000-8000-000000000001")
