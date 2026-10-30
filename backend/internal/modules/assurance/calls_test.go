// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package assurance

import (
	"fmt"
	"testing"
	"time"
)

func standingCall() CallSubject {
	return CallSubject{
		CallID: "0192f0a0-0000-7000-8000-000000000001", Owner: "0192f0a0-0000-7000-8000-000000000002",
		PeriodStart: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), PeriodEnd: time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC),
		ScopeKind: "workspace", CalledMinor: 2_000_000, Currency: "EUR",
		Net: map[string]int64{"open": 0, "evidence": 0},
	}
}

// A call nothing material moved under raises nothing.
func TestACurrentCallRaisesNoFinding(t *testing.T) {
	t.Parallel()
	if f := callFinding(standingCall()); f != nil {
		t.Fatalf("a current call raised %+v", f)
	}
}

// A moved call is one finding about the call, naming the deals, with each
// reading's change kept apart.
func TestAMovedCallIsOneFindingThatNamesItsDeals(t *testing.T) {
	t.Parallel()
	call := standingCall()
	for i := range 25 {
		call.DealIDs = append(call.DealIDs, fmt.Sprintf("deal-%02d", i))
	}
	call.Net = map[string]int64{"open": -400_000, "evidence": -150_000}
	f := callFinding(call)
	if f == nil || f.Type != TypeForecastCallMoved || f.SubjectKind != SubjectKindForecastCall || f.SubjectID != call.CallID {
		t.Fatalf("finding = %+v, want one call finding about the call", f)
	}
	if LogicalKey(*f) != LogicalKey(*callFinding(call)) {
		t.Fatal("the same call raised two identities, so a second night would raise a second finding")
	}
	if got := f.Observed["changed_deals"]; got != 25 {
		t.Errorf("changed_deals = %v, want every moved deal counted", got)
	}
	if named, ok := f.Observed["deal_ids"].([]string); !ok || len(named) != 25 {
		t.Errorf("deal_ids = %v, want every moved deal stored", f.Observed["deal_ids"])
	}
	if f.Observed["open_change_minor"] != int64(-400_000) || f.Observed["evidence_change_minor"] != int64(-150_000) {
		t.Errorf("observed = %v, want each reading's change on its own key", f.Observed)
	}
	if f.AffectedMinor != nil {
		t.Errorf("the finding adds the readings into one amount %d", *f.AffectedMinor)
	}
}

// A call whose basis changed raises a limitation and no delta.
func TestACallThatCannotBeComparedSaysWhy(t *testing.T) {
	t.Parallel()
	call := standingCall()
	call.Limitation = "population_changed"
	f := callFinding(call)
	if f == nil || f.Severity != SeverityLow || f.Observed["limitation"] != "population_changed" {
		t.Fatalf("finding = %+v, want a low finding naming the limitation", f)
	}
	if _, delta := f.Observed["open_change_minor"]; delta {
		t.Error("a limited comparison still reported a change")
	}
}
