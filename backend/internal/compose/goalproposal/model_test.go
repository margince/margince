// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package goalproposal

import (
	"strings"
	"testing"
)

func major(amounts ...int64) []*int64 {
	out := make([]*int64, 12)
	for index := range amounts {
		out[index] = &amounts[index]
	}
	return out
}

// lastYearInput is a seller whose last year closed more in autumn, with the
// plan year not begun.
func lastYearInput() Input {
	return Input{
		Year: 2027, Currency: "EUR", Today: "2026-11-12", Lang: "en",
		Closed: []YearIn{{Year: 2027, Months: major()}, {Year: 2026, Months: major(10000, 10000, 12000, 12000, 8000, 6000, 6000, 8000, 20000, 24000)}},
	}
}

const evenReply = `{"months":[100,100,100,100,100,100,100,100,100,100,100,100],"reason":"The months follow last year's even run."}`

func TestParseReadsTwelveMonthsIntoMinorUnits(t *testing.T) {
	proposal, err := Parse("```json\n"+evenReply+"\n```", lastYearInput())
	if err != nil {
		t.Fatal(err)
	}
	if len(proposal.Months) != 12 || proposal.Months[0] != 10000 || proposal.Total() != 120000 {
		t.Fatalf("proposal = %+v, want twelve months of 100.00 EUR", proposal)
	}
	yen := lastYearInput()
	yen.Currency = "JPY"
	if proposal, err = Parse(evenReply, yen); err != nil || proposal.Months[0] != 100 {
		t.Fatalf("a currency with no minor unit reads %v %v, want 100", proposal.Months, err)
	}
}

func TestParseSpreadsAYearlyGoalOverTheProposedPatternExactly(t *testing.T) {
	in := lastYearInput()
	goal := int64(1000)
	in.YearlyGoal, in.YearlyGoalMinor = &goal, 100001
	proposal, err := Parse(`{"months":[1,1,1,1,1,1,1,1,1,1,1,3],"reason":"December carries the year-end push."}`, in)
	if err != nil {
		t.Fatal(err)
	}
	if proposal.Total() != 100001 {
		t.Fatalf("the months add up to %d, want the yearly goal of 100001 to the minor unit", proposal.Total())
	}
	if proposal.Months[11] <= 2*proposal.Months[0] {
		t.Fatalf("months = %v, want December near three times January", proposal.Months)
	}
}

func TestParseRefusesWhatIsNotAPlan(t *testing.T) {
	for name, reply := range map[string]string{
		"eleven months":      `{"months":[1,1,1,1,1,1,1,1,1,1,1],"reason":"Even."}`,
		"a negative month":   `{"months":[-1,1,1,1,1,1,1,1,1,1,1,1],"reason":"Even."}`,
		"a fractional month": `{"months":[1.5,1,1,1,1,1,1,1,1,1,1,1],"reason":"Even."}`,
		"all zero":           `{"months":[0,0,0,0,0,0,0,0,0,0,0,0],"reason":"Even."}`,
		"no reason":          `{"months":[1,1,1,1,1,1,1,1,1,1,1,1],"reason":" "}`,
		"a figure in reason": `{"months":[1,1,1,1,1,1,1,1,1,1,1,1],"reason":"Autumn closed 24000 last year."}`,
		"a long reason":      `{"months":[1,1,1,1,1,1,1,1,1,1,1,1],"reason":"` + strings.Repeat("even ", 80) + `"}`,
		"not json":           `Here are the months.`,
	} {
		if proposal, err := Parse(reply, lastYearInput()); err == nil {
			t.Errorf("%s was kept as %+v", name, proposal)
		}
	}
}

func TestTheRequestShowsTheClosedRevenueAndTheYearlyGoal(t *testing.T) {
	in := lastYearInput()
	goal := int64(150000)
	in.YearlyGoal = &goal
	content := Request(in).Messages[0].Content
	for _, want := range []string{`"year":2027`, `"yearly_goal":150000`, `{"year":2026,"months":[10000,10000,12000,12000,8000,6000,6000,8000,20000,24000,null,null]}`} {
		if !strings.Contains(content, want) {
			t.Errorf("the request does not show %s in %s", want, content)
		}
	}
	if !in.HasHistory() || (Input{Closed: []YearIn{{Year: 2026, Months: major(0, 0)}}}).HasHistory() {
		t.Error("history is closed revenue above zero in some month, and nothing less")
	}
}
