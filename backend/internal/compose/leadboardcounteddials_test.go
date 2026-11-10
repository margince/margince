// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"maps"
	"os"
	"regexp"
	"slices"
	"testing"
)

// The Leads board's COUNTED_DIALS mirrors this spec's filters. A filter only
// here withholds counts the report could give; one only there answers 422.
const leadBoardQueries = "../../../frontend/src/features/leads/leadboard.queries.ts"

var countedDialsLiteral = regexp.MustCompile(`(?s)const COUNTED_DIALS = \[(.*?)\] as const;`)

var quotedName = regexp.MustCompile(`"([a-z_]+)"`)

func TestTheLeadBoardCountsUnderExactlyTheReportsFilters(t *testing.T) {
	t.Parallel()
	source, err := os.ReadFile(leadBoardQueries)
	if err != nil {
		t.Fatalf("reading the lead board's counted dials: %v", err)
	}
	literal := countedDialsLiteral.FindSubmatch(source)
	if literal == nil {
		t.Fatalf("%s no longer declares COUNTED_DIALS as an array literal: this test reads a shape that is gone", leadBoardQueries)
	}
	var board []string
	for _, m := range quotedName.FindAllSubmatch(literal[1], -1) {
		board = append(board, string(m[1]))
	}
	if len(board) == 0 {
		t.Fatal("no dial parsed out of COUNTED_DIALS: a mirror read as empty agrees with nothing")
	}
	spec, ok := prebuiltReports["leads-by-status"]
	if !ok {
		t.Fatal("the leads-by-status report is gone, and the lead board still asks it for counts")
	}
	report := slices.Sorted(maps.Keys(spec.filters))
	slices.Sort(board)
	if !slices.Equal(board, report) {
		t.Errorf("the lead board counts under %v, and leads-by-status filters by %v; change both together", board, report)
	}
}
