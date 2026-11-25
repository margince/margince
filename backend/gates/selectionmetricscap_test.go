// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// One reporting selection holds at most reporting.MaxSelectionMetrics metrics.
// The contract's `maxItems` on every list of metric ids must say the same. A
// lower Go bound refuses a request the contract calls legal. An Analytics view
// then fails to load with a refusal its reader cannot act on.

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
	caps := contractMetricListCaps(t)
	if len(caps) < selectionMetricLists {
		t.Errorf("found %d bounded lists of metric ids in the contract, want at least %d", len(caps), selectionMetricLists)
	}
	for line, got := range caps {
		if got != want {
			t.Errorf("crm.yaml line %d bounds a metric list at %d and MaxSelectionMetrics is %d", line, got, want)
		}
	}
}

// contractMetricListCaps finds each array whose items are metric ids and that
// declares a maxItems, keyed by the line of that maxItems. The bound sits
// within two lines of the items' $ref, on either side, in every spelling the
// contract uses.
func contractMetricListCaps(t *testing.T) map[int]int {
	t.Helper()
	raw, err := gatekit.ReadFile(contractPath)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(raw), "\n")
	caps := map[int]int{}
	for i, line := range lines {
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
		}
	}
	return caps
}
