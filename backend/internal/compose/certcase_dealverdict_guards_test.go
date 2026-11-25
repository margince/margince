// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose_test

// The cert case scores the model's answer without the pass's guards, so a
// case could expect an amount the pass would always drop. This gate runs
// each expected amount through dealverdict.Decide itself.

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/aicert"
	"github.com/margince/margince/backend/internal/compose/dealverdict"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/deals"
)

// sentenceEnd splits a body into the sentences a model could quote.
var sentenceEnd = regexp.MustCompile(`[.!?:]\s+|\n\s*\n`)

func TestEveryExpectedDealAmountSurvivesThePassGuards(t *testing.T) {
	census, err := compose.NewTaskCensus()
	if err != nil {
		t.Fatalf("building the task census: %v", err)
	}
	scenarios, err := aicert.LoadCorpus("aicert/corpus", census)
	if err != nil {
		t.Fatalf("loading the corpus: %v", err)
	}
	checked := 0
	for _, sc := range scenarios {
		if ai.Task(sc.Task) != ai.TaskDealVerdict {
			continue
		}
		factory, bound := census.CaseFor(ai.Task(sc.Task), sc.Site)
		if !bound {
			t.Fatalf("scenario %s names site %s/%s, which no case is bound to", sc.Name, sc.Task, sc.Site)
		}
		pc, err := factory.Prepare(json.RawMessage(sc.Fixture), json.RawMessage(sc.Expect.Answer))
		if err != nil {
			t.Fatalf("scenario %s: %v", sc.Name, err)
		}
		in, amounts, ok := compose.DealVerdictCaseInput(pc)
		if !ok {
			t.Fatalf("scenario %s: the deal_verdict case is not the one this gate reads", sc.Name)
		}
		for _, amount := range amounts {
			if !amountApplies(t, in, amount) {
				t.Errorf("scenario %s expects set_amount %d, but no quote from its activities passes the pass's guards: "+
					"the price must stand in the customer's own inbound email", sc.Name, amount)
			}
			checked++
		}
	}
	if checked == 0 {
		t.Fatal("no deal_verdict scenario expects set_amount, so the gate measured nothing")
	}
}

// amountApplies reports whether some sentence of some activity, quoted and
// cited as a model would, makes Decide apply the amount.
func amountApplies(t *testing.T, in dealverdict.Input, amount int64) bool {
	t.Helper()
	in.ReadAt = in.Today.AddDate(0, 0, 1)
	state := dealverdict.State{
		Seen:       deals.VerdictValues{AmountMinor: in.AmountMinor, Currency: in.Currency},
		SwitchedOn: map[string]bool{deals.VerdictAmountCorrection: true},
	}
	for _, activity := range in.Activities {
		for _, sentence := range sentenceEnd.Split(activity.Body, -1) {
			action := dealverdict.Action{
				Kind: dealverdict.KindSetAmount, Amount: amount, Currency: *in.Currency,
				QuotedWords: strings.TrimSpace(sentence), Cited: []string{activity.ID}, Confidence: 1, Reason: "gate",
			}
			decisions, err := dealverdict.Decide(in, state, dealverdict.Payload{Actions: []dealverdict.Action{action}}, nil)
			if err != nil {
				t.Fatalf("deciding an amount: %v", err)
			}
			if decisions[0].Outcome == dealverdict.OutcomeApply {
				return true
			}
		}
	}
	return false
}
