// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

import (
	"os"
	"strings"
	"testing"
)

// The kept question answers reach a subject's leads through leadTwinOf, and
// erasure wipes them through its own statement. A lead one finds and the
// other misses is a kept answer erasure leaves behind.
func TestTheReportingReachFindsTheLeadsErasureWipes(t *testing.T) {
	source, err := os.ReadFile("erasure_leadtwins.go")
	if err != nil {
		t.Fatal(err)
	}
	squash := func(text string) string { return strings.Join(strings.Fields(text), " ") }
	if !strings.Contains(squash(string(source)), "WHERE "+squash(leadTwinOf("$1", "$2"))+" RETURNING id") {
		t.Fatal("anonymizeLeadTwins no longer wipes exactly the leads leadTwinOf selects; change both together")
	}
}
