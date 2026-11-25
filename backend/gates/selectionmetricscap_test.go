// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// One reporting selection holds at most reporting.MaxSelectionMetrics metrics.
// The contract's ReportingSelection must say the same, or a view the contract
// allows is refused. No other metric list may exceed it, because each one is
// later sent as a selection.

import (
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const (
	selectionMetricsOwner = "internal/modules/reporting/reports.go"
	// selectionMetricLists is how many bounded metric lists the contract has
	// today. A floor: fewer means the scan stopped seeing them.
	selectionMetricLists = 4
	// selectionSchema is the selection the evaluation validates; its bound is
	// the one a view is measured against.
	selectionSchema = "    ReportingSelection:"
)

var (
	selectionMetricsConst = regexp.MustCompile(`(?m)^const MaxSelectionMetrics = (\d+)$`)
	metricIDItems         = regexp.MustCompile(`\$ref: '#/components/schemas/ReportingMetricID'`)
	contractMaxItems      = regexp.MustCompile(`^\s*maxItems: (\d+)\s*$`)
)

func TestTheSelectionMetricsCapMatchesTheContract(t *testing.T) {
	t.Parallel()
	src, err := gatekit.ReadFile(selectionMetricsOwner)
	if err != nil {
		t.Fatal(err)
	}
	match := selectionMetricsConst.FindSubmatch(src)
	if match == nil {
		t.Fatalf("%s no longer declares MaxSelectionMetrics as a single const; restate it so this gate can read it",
			selectionMetricsOwner)
	}
	want, err := strconv.Atoi(string(match[1]))
	if err != nil {
		t.Fatal(err)
	}
	caps, selectionLine := contractMetricListCaps(t)
	if len(caps) < selectionMetricLists {
		t.Errorf("found %d bounded lists of metric ids in the contract, want at least %d", len(caps), selectionMetricLists)
	}
	if caps[selectionLine] != want {
		t.Errorf("ReportingSelection.metrics allows %d (crm.yaml line %d) and MaxSelectionMetrics is %d", caps[selectionLine], selectionLine, want)
	}
	for line, got := range caps {
		if got > want {
			t.Errorf("crm.yaml line %d bounds a metric list at %d, above MaxSelectionMetrics %d", line, got, want)
		}
	}
}

// contractMetricListCaps keys each bounded list of metric ids by the line of
// its maxItems. It also returns the line of the first one in ReportingSelection.
// The bound sits within two lines of the items' $ref, in every spelling used.
func contractMetricListCaps(t *testing.T) (map[int]int, int) {
	t.Helper()
	raw, err := gatekit.ReadFile(contractPath)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(raw), "\n")
	caps := map[int]int{}
	selectionStart, selectionLine := -1, 0
	for i, line := range lines {
		if line == selectionSchema {
			selectionStart = i
		}
		if !metricIDItems.MatchString(line) {
			continue
		}
		for j := max(i-2, 0); j < min(i+3, len(lines)); j++ {
			found := contractMaxItems.FindStringSubmatch(lines[j])
			if found == nil {
				continue
			}
			value, convErr := strconv.Atoi(found[1])
			if convErr != nil {
				t.Fatal(convErr)
			}
			caps[j+1] = value
			if selectionStart >= 0 && selectionLine == 0 {
				selectionLine = j + 1
			}
		}
	}
	if selectionStart < 0 || selectionLine == 0 {
		t.Fatal("crm.yaml has no bounded metric list in ReportingSelection")
	}
	return caps, selectionLine
}
