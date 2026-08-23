// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"os"
	"slices"
	"strings"
	"testing"
)

// The capture principal's grants, as the connector path actually carries them:
// activity, person, organization, project, deal — and NO relationship grant,
// because reading edges is not what capture does.
//
// This is the product's own posture written down, not a preference — and it is
// a slice rather than a reason map, so it carries no gatekit:fixture marker:
// that census enumerates map-to-string declarations, and a marker anywhere else
// classifies nothing.
var captureGrants = []string{"activity", "person", "organization", "project", "deal"}

// The project-attribution ladder's candidate walk must reach a project with the
// grants above and nothing more.
//
// It reads the `relationship` table to find which projects a message's account
// is on, and an edge read normally carries auth.EdgeReadScope. That gate refused
// EVERY candidate for EVERY connector, because a connector holds no relationship
// grant — and the ladder answering "nothing to propose" is indistinguishable
// from the feature working, so no unit test noticed and main went red on the
// integration lane instead.
//
// The rule this holds: the walk may not ask for an object the capture principal
// does not hold. A gate added here later fails this test rather than the
// product.
func TestTheAttributionLadderAsksOnlyForGrantsCaptureHolds(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("projectattribution.go")
	if err != nil {
		t.Fatalf("reading the ladder's source: %v", err)
	}
	source := string(raw)
	walk := functionBody(t, source, "LiveProjectsReached")

	for _, asked := range []struct{ call, object string }{
		{"auth.EdgeReadScope", "relationship"},
		{"auth.EdgeReadAdmitted", "relationship"},
		{`auth.Require(ctx, "relationship"`, "relationship"},
	} {
		if strings.Contains(walk, asked.call) && !slices.Contains(captureGrants, asked.object) {
			t.Errorf("the candidate walk calls %s, which asks for %q — a grant the capture "+
				"principal does not hold, so every connector's candidates are refused and the "+
				"ladder goes silent. The edge is the PATH here, not the answer: the project id it "+
				"yields is staged as an approval that re-checks project.read against the deciding "+
				"human.", asked.call, asked.object)
		}
	}
}

// functionBody is the source of one function, from its declaration to the next
// one — enough to see which gates it calls.
func functionBody(t *testing.T, source, name string) string {
	t.Helper()
	start := strings.Index(source, ") "+name+"(")
	if start < 0 {
		t.Fatalf("no function %q in the source; the test is reading the wrong file", name)
	}
	rest := source[start:]
	if end := strings.Index(rest[1:], "\nfunc "); end > 0 {
		return rest[:end]
	}
	return rest
}
