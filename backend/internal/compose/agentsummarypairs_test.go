// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"os"
	"strings"
	"testing"
)

// The notification panel recognises a subject that is only these pairs
// (frontend/src/app/noticeheadline.ts). Both sides read the same fixture, so a
// changed separator or pair shape fails here and in noticeheadline.test.ts.
func TestSummaryFieldPairsMatchTheFixtureThePanelParses(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../../../frontend/src/app/noticepairs.fixture.txt")
	if err != nil {
		t.Fatalf("reading the fixture: %v", err)
	}
	want := strings.Split(strings.TrimSpace(string(raw)), "\n")
	bodies := []string{
		`{"into_tag_id":"01a11ecb-0000-7000-8000-000000000001"}`,
		`{"won_without_contract_reason":"other","to_stage_id":"01a0ced5-0000-7000-8000-000000000002"}`,
	}
	if len(want) != len(bodies) {
		t.Fatalf("the fixture holds %d lines for %d bodies", len(want), len(bodies))
	}
	for i, body := range bodies {
		got := strings.Join(summaryFields(inEnglish, []byte(body)), ", ")
		if got != want[i] {
			t.Errorf("body %d renders %q, the fixture holds %q", i, got, want[i])
		}
	}
}
