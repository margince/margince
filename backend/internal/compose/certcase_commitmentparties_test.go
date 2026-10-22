// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/aitasks"
)

const certReportedParticipants = `[
 {"key":"mira","kind":"seat","role":"from","name":"Mira"},
 {"key":"ines","kind":"contact","role":"to","name":"Ines"}]`

func TestMailCertificationRejectsAValidButWrongPromiser(t *testing.T) {
	fixture := json.RawMessage(`[{"direction":"outbound","body":"You agreed to pay.","participants":` + certReportedParticipants + `}]`)
	prepared, err := (signalExtractCases{}).Prepare(fixture, json.RawMessage(`[{"kind":"commitment_made","message":1,"promiser":"ines"}]`))
	if err != nil {
		t.Fatal(err)
	}
	c, ok := prepared.(*signalExtractCase)
	if !ok {
		t.Fatal("unexpected signal certification case")
	}
	message := c.thread.Messages[0]
	for _, party := range message.Parties {
		reply := fmt.Sprintf(`{"events":[{"kind":"commitment_made","message_id":%q,"summary":"Pay","quote":"You agreed to pay.","due_date":"","promiser_ref":%q,"confidence":0.95,"commitment":{"state":"open","condition":"","deadline_precision":"none","deadline_quote":""}}]}`, message.ID.String(), party.Ref)
		trace, err := prepared.Run(t.Context(), fixedCompleter{reply: reply})
		if err != nil {
			t.Fatal(err)
		}
		want := aitasks.OutcomeWrongAnswer
		if party.Contact != nil {
			want = aitasks.OutcomeAccepted
		}
		got := prepared.Evaluate(trace)
		if strings.Contains(fmt.Sprint(got), party.Ref) {
			t.Fatal("certification diagnostics contain a minted participant ID")
		}
		if got.Result != want {
			t.Fatalf("%s actor: %+v, want %s", party.Role, got, want)
		}
	}
}

func TestTranscriptCertificationRejectsAValidButWrongPromiser(t *testing.T) {
	fixture := json.RawMessage(`{"lines":["Mira: Ines agreed to pay."],"participants":` + certReportedParticipants + `}`)
	prepared, err := (transcriptProposeCases{}).Prepare(fixture, json.RawMessage(`[{"line":1,"promiser":"ines"}]`))
	if err != nil {
		t.Fatal(err)
	}
	c, ok := prepared.(*transcriptProposeCase)
	if !ok {
		t.Fatal("unexpected transcript certification case")
	}
	for _, party := range c.parties {
		reply := fmt.Sprintf(`{"proposals":[{"summary":"Pay","owner":"Ines","promiser_ref":%q,"due_date":"","source_lines":[1],"confidence":0.95,"commitment":{"state":"open","condition":"","deadline_precision":"none","deadline_quote":""}}]}`, party.Ref)
		trace, err := prepared.Run(t.Context(), fixedCompleter{reply: reply})
		if err != nil {
			t.Fatal(err)
		}
		want := aitasks.OutcomeWrongAnswer
		if party.Contact != nil {
			want = aitasks.OutcomeAccepted
		}
		got := prepared.Evaluate(trace)
		if strings.Contains(fmt.Sprint(got), party.Ref) {
			t.Fatal("certification diagnostics contain a minted participant ID")
		}
		if got.Result != want {
			t.Fatalf("%s actor: %+v, want %s", party.Role, got, want)
		}
	}
}

func TestTranscriptCertificationGradesDatesExactly(t *testing.T) {
	for _, wanted := range []string{"", "2026-03-06", "2026-03-11"} {
		for _, answer := range []string{"", "2026-03-06", "2026-03-11"} {
			source, precision, quote := "Mira: I will send pricing.", "none", ""
			if answer != "" {
				source, precision, quote = "Mira: I will send pricing on "+answer+".", "day", answer
			}
			c := transcriptProposeCase{lines: []string{source}, expected: []transcriptProposeExpectation{{Line: 1, DueDate: wanted}}}
			trace := aitasks.Trace{Output: fmt.Sprintf(`{"proposals":[{"summary":"Send pricing","owner":"Mira","promiser_ref":"","due_date":%q,"source_lines":[1],"confidence":0.95,"commitment":{"state":"open","condition":"","deadline_precision":%q,"deadline_quote":%q}}]}`, answer, precision, quote)}
			got := c.Evaluate(trace)
			if (got.Result == aitasks.OutcomeAccepted) != (answer == wanted) {
				t.Fatalf("wanted date %q, answer %q: %+v", wanted, answer, got)
			}
		}
	}
}
