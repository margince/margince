// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H1

package gates

// The queue's recorded verdicts live exactly as long as the figure that reads them.
//
// A deal_risk_day row, with the verdicts under it, exists to answer the same-day next-step figure on
// GET /worklist/response, which reads at most responseWindowMaxDays back. The
// seeded retention window is that number spelled a second time, in SQL, in
// another module. Kept shorter, the widest window counts days whose verdicts
// were already deleted and reports a rate over part of what it names; kept
// longer, rows sit on disk answering nothing. Both directions fail here.
//
// Read from source rather than imported: gates may not depend on a module.

import (
	"os"
	"regexp"
	"strconv"
	"testing"
)

var responseWindowMaxRE = regexp.MustCompile(`(?m)^const responseWindowMaxDays = (\d+)$`)

// widestResponseWindow reads the figure's own ceiling off the projection.
func widestResponseWindow(t *testing.T) int {
	t.Helper()
	src, err := os.ReadFile("internal/compose/attention/waitinglane.go")
	if err != nil {
		t.Fatalf("reading the response projection: %v", err)
	}
	m := responseWindowMaxRE.FindSubmatch(src)
	if m == nil {
		t.Fatal("no `const responseWindowMaxDays = N` in waitinglane.go — the ceiling moved and this gate compares against nothing")
	}
	days, err := strconv.Atoi(string(m[1]))
	if err != nil {
		t.Fatalf("responseWindowMaxDays is not a number: %v", err)
	}
	return days
}

// seededVerdictWindow reads the retain_days SeedDefaultRetentionTx plants for
// the verdict table.
func seededVerdictWindow(t *testing.T) int {
	t.Helper()
	src, err := os.ReadFile("internal/modules/consent/retention.go")
	if err != nil {
		t.Fatalf("reading the seed: %v", err)
	}
	for _, m := range seedRowRE.FindAllStringSubmatch(string(src), -1) {
		if m[1] != "deal_risk_day" || m[2] != "" {
			continue
		}
		days, err := strconv.Atoi(m[3])
		if err != nil {
			t.Fatalf("the verdict row's retain_days is not a number: %v", err)
		}
		return days
	}
	t.Fatal("SeedDefaultRetentionTx plants no ('deal_risk_day', NULL, …) row — the verdicts would age on nobody's clock")
	return 0
}

func TestTheVerdictWindowIsTheFiguresWidestWindow(t *testing.T) {
	t.Parallel()
	widest, seeded := widestResponseWindow(t), seededVerdictWindow(t)
	if widest != seeded {
		t.Errorf("deal_risk_day is seeded to age out after %d days, and GET /worklist/response reads up to %d days back.\n"+
			"The row exists only to answer that figure, so the two are one number: move the seed in "+
			"consent/retention.go to %d, or the ceiling in attention/waitinglane.go to %d.", seeded, widest, widest, seeded)
	}
}
