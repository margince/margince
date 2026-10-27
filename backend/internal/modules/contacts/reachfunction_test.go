// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// A department wins over a rank. "Head of IT" names whom a rep is introduced
// to, and a rank alone names no function.
func TestAJobTitleNamesTheFunctionItWorksIn(t *testing.T) {
	t.Parallel()
	for title, want := range map[string]crmcontracts.LinkedInReachFunction{
		"Head of IT":                   crmcontracts.LinkedInReachFunctionIt,
		"IT-Leiter":                    crmcontracts.LinkedInReachFunctionIt,
		"Senior Software Engineer":     crmcontracts.LinkedInReachFunctionIt,
		"CFO":                          crmcontracts.LinkedInReachFunctionFinance,
		"Leiterin Finanzbuchhaltung":   crmcontracts.LinkedInReachFunctionFinance,
		"Key Account Manager":          crmcontracts.LinkedInReachFunctionSales,
		"Vertriebsleiter DACH":         crmcontracts.LinkedInReachFunctionSales,
		"Marketing Lead":               crmcontracts.LinkedInReachFunctionMarketing,
		"Head of HR":                   crmcontracts.LinkedInReachFunctionHr,
		"Personalleiterin":             crmcontracts.LinkedInReachFunctionHr,
		"Head of People & Culture":     crmcontracts.LinkedInReachFunctionHr,
		"Strategic Buyer":              crmcontracts.LinkedInReachFunctionProcurement,
		"Leiter Einkauf":               crmcontracts.LinkedInReachFunctionProcurement,
		"Chief Operating Officer":      crmcontracts.LinkedInReachFunctionOperations,
		"Werkleiter":                   crmcontracts.LinkedInReachFunctionOperations,
		"Managing Director":            crmcontracts.LinkedInReachFunctionManagement,
		"Geschäftsführerin":            crmcontracts.LinkedInReachFunctionManagement,
		"Co-Founder & CEO":             crmcontracts.LinkedInReachFunctionManagement,
		"CEO and Head of Sales":        crmcontracts.LinkedInReachFunctionSales,
		"Personal Assistant to the MD": "",
		"Director":                     "",
		"Head of":                      "",
		"":                             "",
	} {
		got, ok := reachFunctionOf(title)
		if want == "" {
			if ok {
				t.Errorf("%q names %s, want no function", title, got)
			}
			continue
		}
		if !ok || got != want {
			t.Errorf("%q names %q (found %v), want %s", title, got, ok, want)
		}
	}
}

func TestAFunctionHeldByOneConnectionIsLeftOut(t *testing.T) {
	t.Parallel()
	got := reachFunctionCounts(map[crmcontracts.LinkedInReachFunction]int{
		crmcontracts.LinkedInReachFunctionIt:      3,
		crmcontracts.LinkedInReachFunctionFinance: 2,
		crmcontracts.LinkedInReachFunctionSales:   2,
		crmcontracts.LinkedInReachFunctionHr:      1,
	})
	want := []crmcontracts.LinkedInReachFunctionCount{
		{Function: crmcontracts.LinkedInReachFunctionIt, Connections: 3},
		{Function: crmcontracts.LinkedInReachFunctionFinance, Connections: 2},
		{Function: crmcontracts.LinkedInReachFunctionSales, Connections: 2},
	}
	if len(got) != len(want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("position %d is %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestATallyCountsEveryConnectionAndOnlyMatchedTitlesAsFunctions(t *testing.T) {
	t.Parallel()
	tally := reachTally{functions: map[crmcontracts.LinkedInReachFunction]int{}}
	for _, title := range []string{"CIO", "IT Manager", "Director", ""} {
		tally.add(title)
	}
	if tally.connections != 4 {
		t.Errorf("counted %d connections, want 4: a title no function matches is still a connection", tally.connections)
	}
	if tally.functions[crmcontracts.LinkedInReachFunctionIt] != 2 || len(tally.functions) != 1 {
		t.Errorf("functions are %v, want it: 2 and nothing else", tally.functions)
	}
}
