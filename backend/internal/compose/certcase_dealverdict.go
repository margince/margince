// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The certification case for deal_verdict/deal_verdict. The request comes
// from dealverdict.Request and the reply is read by dealverdict.Parse, as in
// the nightly pass.
//
// The expectation is the change kinds a correct answer proposes. Prepare
// mints the activity ids, so a cited id is one the model was handed.

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/compose/dealverdict"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

type dealVerdictFixture struct {
	Today      string                       `json:"today"`
	Deal       dealVerdictDealFixture       `json:"deal"`
	Activities []dealVerdictActivityFixture `json:"activities"`
}

type dealVerdictDealFixture struct {
	Name        string `json:"name"`
	Stage       string `json:"stage"`
	Amount      int64  `json:"amount"`
	Currency    string `json:"currency"`
	CloseDate   string `json:"close_date"`
	Probability int    `json:"probability"`
	// ProbabilitySource says who set Probability, the stage unless named.
	ProbabilitySource string `json:"probability_source"`
	// StageProbability is the stage's own, which a raise may reach. Unset, the
	// deal's Probability is already the limit.
	StageProbability int `json:"stage_probability"`
	// Later are the open stages after the deal's, in order. ExitCriteria
	// belong to the deal's own stage unless they name a later one.
	Later        []dealVerdictStageFixture     `json:"later"`
	ExitCriteria []dealVerdictCriterionFixture `json:"exit_criteria"`
}

type dealVerdictStageFixture struct {
	Name        string `json:"name"`
	Probability int    `json:"probability"`
}

type dealVerdictCriterionFixture struct {
	Stage string `json:"stage"`
	Label string `json:"label"`
	Kind  string `json:"kind"`
}

type dealVerdictActivityFixture struct {
	Kind      string `json:"kind"`
	Direction string `json:"direction"`
	At        string `json:"at"`
	Subject   string `json:"subject"`
	Body      string `json:"body"`
}

// dealVerdictExpectation is one change a correct answer proposes. Date and
// Amount are checked only when set.
type dealVerdictExpectation struct {
	Kind   string `json:"kind"`
	Date   string `json:"date,omitempty"`
	Amount int64  `json:"amount,omitempty"`
	// Stage is the name of the stage a move goes to.
	Stage string `json:"stage,omitempty"`
}

type dealVerdictCases struct{}

func (dealVerdictCases) Site() aitasks.Site {
	return aitasks.Site{Task: ai.TaskDealVerdict, Variant: "deal_verdict", Kind: ai.SiteKindOneShot}
}

// CertifiedScope is the whole invocation: the pass makes one call per deal
// and acts on its answer alone.
func (dealVerdictCases) CertifiedScope() string { return aitasks.ScopeFullInvocation }

// Prepare builds the input the pass would build for the fixture's deal.
//
//nolint:ireturn // PreparedCase IS the seam: one implementation per site behind the one interface the cert lane runs.
func (dealVerdictCases) Prepare(fixture, expected json.RawMessage) (aitasks.PreparedCase, error) {
	var f dealVerdictFixture
	if err := json.Unmarshal(fixture, &f); err != nil {
		return nil, fmt.Errorf("deal_verdict/deal_verdict: the fixture is not the shape this site takes: %w", err)
	}
	var want []dealVerdictExpectation
	if err := json.Unmarshal(expected, &want); err != nil {
		return nil, fmt.Errorf("deal_verdict/deal_verdict: the expected answer is not a list of changes: %w", err)
	}
	in, err := dealVerdictFixtureInput(f)
	if err != nil {
		return nil, err
	}
	return &dealVerdictCase{in: in, expected: want}, nil
}

func dealVerdictFixtureInput(f dealVerdictFixture) (dealverdict.Input, error) {
	today, err := time.Parse(time.DateOnly, f.Today)
	if err != nil || len(f.Activities) == 0 {
		return dealverdict.Input{}, errors.New("deal_verdict/deal_verdict: the fixture needs a today date and at least one activity")
	}
	amount, currency := f.Deal.Amount*100, f.Deal.Currency
	in := dealverdict.Input{
		Today: today, DealName: f.Deal.Name, Stage: f.Deal.Stage, AmountMinor: &amount, Currency: &currency,
		Probability: f.Deal.Probability, StageProbability: f.Deal.StageProbability,
		ProbabilitySource: cmp.Or(f.Deal.ProbabilitySource, string(deals.SourceStage)),
	}
	if closeDate, err := time.Parse(time.DateOnly, f.Deal.CloseDate); err == nil {
		in.CloseDate = &closeDate
	}
	stageFixtureInput(f.Deal, &in)
	for _, a := range f.Activities {
		at, err := time.Parse(time.DateOnly, a.At)
		if err != nil {
			return dealverdict.Input{}, fmt.Errorf("deal_verdict/deal_verdict: activity date %q is not YYYY-MM-DD", a.At)
		}
		in.Activities = append(in.Activities, dealverdict.Activity{
			ID: ids.NewV7().String(), Kind: a.Kind, Direction: a.Direction, OccurredAt: at, Subject: a.Subject, Body: a.Body,
		})
	}
	return in, nil
}

// stageFixtureInput places the deal's stage at position 1 and the later
// stages after it, each criterion on the stage it names.
func stageFixtureInput(deal dealVerdictDealFixture, in *dealverdict.Input) {
	in.StagePosition = 1
	positions := map[string]int{deal.Stage: 1}
	for i, stage := range deal.Later {
		positions[stage.Name] = i + 2
		in.Later = append(in.Later, dealverdict.StageOption{
			ID: ids.NewV7().String(), Name: stage.Name, Position: i + 2, Probability: stage.Probability,
		})
	}
	for _, c := range deal.ExitCriteria {
		stage := cmp.Or(c.Stage, deal.Stage)
		in.ExitCriteria = append(in.ExitCriteria, dealverdict.ExitCriterion{
			ID: ids.NewV7().String(), StagePosition: positions[stage], Stage: stage, Label: c.Label, Kind: c.Kind,
		})
	}
}

type dealVerdictCase struct {
	in       dealverdict.Input
	expected []dealVerdictExpectation
}

// Run issues the one request the pass sends, without the shape retry.
func (c *dealVerdictCase) Run(ctx context.Context, completer aitasks.Completer) (aitasks.Trace, error) {
	req := dealverdict.Request(c.in, "en")
	trace := aitasks.Trace{Requests: []model.Request{req}}
	resp, err := completer.Complete(ctx, req)
	if err != nil {
		return trace, fmt.Errorf("deal_verdict/deal_verdict: %w", err)
	}
	trace.Output = resp.Text
	return trace, nil
}

// Evaluate parses with the pass's own check, then compares the proposed
// kinds with the scenario's. The guards are the pass's decision about an
// answer it believes, so they are not applied here.
func (c *dealVerdictCase) Evaluate(trace aitasks.Trace) aitasks.Outcome {
	payload, err := dealverdict.Parse(trace.Output, c.in)
	if err != nil {
		return aitasks.Outcome{Result: aitasks.OutcomeInvalid, Detail: err.Error()}
	}
	if faults := dealVerdictFaults(payload, c.expected, c.in); len(faults) > 0 {
		return aitasks.Outcome{Result: aitasks.OutcomeWrongAnswer, Detail: strings.Join(faults, "; ")}
	}
	return aitasks.Outcome{Result: aitasks.OutcomeAccepted}
}

func dealVerdictFaults(payload dealverdict.Payload, want []dealVerdictExpectation, in dealverdict.Input) []string {
	var faults []string
	var proposed []string
	for _, action := range payload.Actions {
		proposed = append(proposed, action.Kind)
		i := slices.IndexFunc(want, func(w dealVerdictExpectation) bool { return w.Kind == action.Kind })
		if i < 0 {
			faults = append(faults, fmt.Sprintf("proposed %s, which the evidence does not support", action.Kind))
			continue
		}
		if want[i].Date != "" && action.Date != want[i].Date {
			faults = append(faults, fmt.Sprintf("set the date to %s, want %s", action.Date, want[i].Date))
		}
		if want[i].Amount != 0 && action.Amount != want[i].Amount {
			faults = append(faults, fmt.Sprintf("set the amount to %d, want %d", action.Amount, want[i].Amount))
		}
		if want[i].Stage != "" && stageNamed(in, action.StageID) != want[i].Stage {
			faults = append(faults, fmt.Sprintf("moved to %q, want %q", stageNamed(in, action.StageID), want[i].Stage))
		}
	}
	for _, w := range want {
		if !slices.Contains(proposed, w.Kind) {
			faults = append(faults, fmt.Sprintf("did not propose %s", w.Kind))
		}
	}
	return faults
}

func stageNamed(in dealverdict.Input, id string) string {
	for _, stage := range in.Later {
		if stage.ID == id {
			return stage.Name
		}
	}
	return ""
}
