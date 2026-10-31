// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package assurance

import (
	"reflect"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/viability"
)

func TestEachRiskSignalCarriesTheFactsBehindIt(t *testing.T) {
	t.Parallel()
	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	asOf := time.Date(2026, 10, 31, 9, 0, 0, 0, berlin)
	day := func(d int) *time.Time {
		at := time.Date(2026, 10, d, 0, 0, 0, 0, time.UTC)
		return &at
	}
	stuckAfter := 10 * 24 * time.Hour
	cases := []struct {
		name    string
		subject Subject
		want    []RiskSignal
	}{
		{
			"a close date that keeps moving and has passed",
			Subject{CloseDatePushes: 3, ExpectedClose: day(29), HasNextStep: true},
			[]RiskSignal{{Code: SignalClosePushed, Count: new(3), ClosePassed: true}},
		},
		{
			"a close date that keeps moving and is ahead",
			Subject{CloseDatePushes: 4, ExpectedClose: new(time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)), HasNextStep: true},
			[]RiskSignal{{Code: SignalClosePushed, Count: new(4)}},
		},
		{
			"no next step and the close tomorrow",
			Subject{ExpectedClose: new(time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC))},
			[]RiskSignal{{Code: SignalNoNextStep, ClosesInDays: new(1)}},
		},
		{
			"no next step and the close weeks away",
			Subject{ExpectedClose: new(time.Date(2026, 12, 15, 0, 0, 0, 0, time.UTC))},
			[]RiskSignal{{Code: SignalNoNextStep}},
		},
		{
			"a quiet customer",
			Subject{HasNextStep: true, Viability: &SubjectViability{Reasons: []viability.Reason{viability.ReasonCustomerSilent}, SilentDays: 34}},
			[]RiskSignal{{Code: SignalQuiet, Days: new(34)}},
		},
		{
			"stalled in its stage",
			Subject{HasNextStep: true, StageID: "stage-1", StageEnteredAt: time.Date(2026, 10, 10, 15, 0, 0, 0, berlin)},
			[]RiskSignal{{Code: SignalStuck, Days: new(21), StageID: "stage-1"}},
		},
		{
			"in its stage no longer than won deals were",
			Subject{HasNextStep: true, StageID: "stage-1", StageEnteredAt: time.Date(2026, 10, 25, 9, 0, 0, 0, berlin)},
			[]RiskSignal{},
		},
	}
	for _, c := range cases {
		got := RiskSignals(asOf, c.subject, DefaultConfig(), &stuckAfter, berlin)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %+v, want %+v", c.name, got, c.want)
		}
	}
}

func TestAStageWonDealsClickedThroughIsNotStuckOnTheDayADealEnteredIt(t *testing.T) {
	t.Parallel()
	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	asOf := time.Date(2026, 10, 31, 15, 0, 0, 0, berlin)
	stuckAfter := 4 * time.Second
	enteredThisMorning := Subject{HasNextStep: true, StageID: "stage-1", StageEnteredAt: time.Date(2026, 10, 31, 9, 0, 0, 0, berlin)}
	if got := RiskSignals(asOf, enteredThisMorning, DefaultConfig(), &stuckAfter, berlin); len(got) != 0 {
		t.Errorf("a deal in its stage since this morning: got %+v, want no signal", got)
	}
	enteredYesterday := Subject{HasNextStep: true, StageID: "stage-1", StageEnteredAt: time.Date(2026, 10, 30, 17, 0, 0, 0, berlin)}
	want := []RiskSignal{{Code: SignalStuck, Days: new(1), StageID: "stage-1"}}
	if got := RiskSignals(asOf, enteredYesterday, DefaultConfig(), &stuckAfter, berlin); !reflect.DeepEqual(got, want) {
		t.Errorf("a deal in its stage since yesterday: got %+v, want %+v", got, want)
	}
}
