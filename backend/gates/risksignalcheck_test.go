// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H2

package gates

// A frozen forecast row accepts the same risk signals the assessment can
// answer. A signal added to assurance alone aborts every snapshot that freezes
// it; one added to the CHECK alone is a code no reader words. Both sides are
// read from their owners: the CHECK from the head catalog, the list from
// assurance.RiskSignalNames().

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/assurance"
)

const riskSignalsCheck = "public.forecast_contribution.forecast_contribution_risk_signals_known CHECK"

func TestTheFrozenRiskSignalCheckAcceptsTheAssessmentsSignals(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile(filepath.Join(repoRoot, "backend", "migrations", "testdata", "head_catalog.txt"))
	if err != nil {
		t.Fatalf("reading the head catalog: %v", err)
	}
	var constrained []string
	for line := range strings.SplitSeq(string(raw), "\n") {
		if !strings.HasPrefix(line, riskSignalsCheck) {
			continue
		}
		for _, match := range checkedLiteral.FindAllStringSubmatch(line, -1) {
			constrained = append(constrained, match[1])
		}
	}
	if len(constrained) == 0 {
		t.Fatal("the head catalog records no risk signal list on forecast_contribution; " +
			"either the CHECK was dropped or renamed, and a parity check with nothing on one side cannot pass")
	}
	declared := assurance.RiskSignalNames()
	if len(declared) == 0 {
		t.Fatal("assurance.RiskSignalNames() is empty, so no signal could be checked")
	}
	slices.Sort(constrained)
	declared = slices.Sorted(slices.Values(declared))
	if !slices.Equal(constrained, declared) {
		t.Errorf("forecast_contribution accepts the risk signals %v and assurance answers %v.\n"+
			"A new migration must widen or narrow forecast_contribution_risk_signals_known with the list, "+
			"or a snapshot freezing the new signal fails.", constrained, declared)
	}
}
