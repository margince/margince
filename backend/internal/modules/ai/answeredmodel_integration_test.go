// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package ai

// Every reader that prices a call prices it at the model that answered. A served
// model the sheet does not price falls back to the binding's rate.

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const (
	boundGemini    = "gemini-3.5-flash"
	answeredGemini = "gemini-3.6-flash"
)

// answeredCase is one call recorded under the bound model, and the rate its
// cost must come from.
type answeredCase struct {
	name                       string
	servedModel, servedSource  string
	wantInputRate, wantOutRate int64
}

var answeredCases = []answeredCase{
	{"the provider named a priced model", answeredGemini, servedIdentitySourceResponse, 750_000, 3_750_000},
	{"the provider named a model the sheet does not price", answeredGemini + "-001", servedIdentitySourceResponse, 1_500_000, 9_000_000},
	{"the wire only echoed the request", answeredGemini, servedIdentitySourceEcho, 1_500_000, 9_000_000},
	{"nobody said which model answered", "", servedIdentitySourceConfigured, 1_500_000, 9_000_000},
}

type answeredFixture struct {
	ws  ids.UUID
	db  *database.DB
	ctx context.Context
	// tasks and correlations are answeredCases' own, index for index.
	tasks        []Task
	correlations []ids.UUID
}

// seedAnsweredCalls prices the bound and the answered model differently and
// records one call per case through the meter, each under its own task.
func seedAnsweredCalls(t *testing.T) answeredFixture {
	t.Helper()
	env := setupRateStore(t)
	ctx := context.Background()
	ws, ctx := env.seedWorkspace(ctx, t)
	db := env.dbFor(ws)
	yesterday := time.Now().AddDate(0, 0, -1)
	env.insertRate(ctx, t, ModelRate{Provider: providerGemini, ModelID: boundGemini, InputPerMTokMicroUSD: 1_500_000, OutputPerMTokMicroUSD: 9_000_000, EffectiveDate: yesterday})
	env.insertRate(ctx, t, ModelRate{Provider: providerGemini, ModelID: answeredGemini, InputPerMTokMicroUSD: 750_000, OutputPerMTokMicroUSD: 3_750_000, EffectiveDate: yesterday})
	f := answeredFixture{ws: ws, db: db, ctx: ctx}
	for _, c := range answeredCases {
		task, correlation := Task("answered_"+ids.NewV7().String()[24:]), ids.NewV7()
		f.tasks, f.correlations = append(f.tasks, task), append(f.correlations, correlation)
		if err := NewCallMeter(db).Record(ctx, []Call{{
			LogicalCallID: ids.NewV7(), Attempt: 1, IsTerminal: true, Kind: callKindCompletion, CorrelationID: &correlation,
			Task: task, Tier: TierPremium, Provider: providerGemini, ModelID: boundGemini, RequestFingerprint: "fp-" + ids.NewV7().String(),
			TokensIn: 1000, TokensOut: 200, ServedModel: c.servedModel, ServedIdentitySource: c.servedSource,
		}}); err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
	}
	return f
}

func (c answeredCase) wantCost() int64 {
	return PriceCall(Usage{TokensIn: 1000, TokensOut: 200}, ModelRate{InputPerMTokMicroUSD: c.wantInputRate, OutputPerMTokMicroUSD: c.wantOutRate})
}

func TestTheCostReportPricesACallAtTheModelThatAnswered(t *testing.T) {
	f := seedAnsweredCalls(t)
	report, err := NewRateStore(f.db).CostReport(f.ctx, time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	byTask := map[Task]DayCost{}
	for _, line := range report {
		byTask[line.Task] = line
	}
	for i, c := range answeredCases {
		if got := byTask[f.tasks[i]]; got.CostMicroUSD != c.wantCost() || got.UnpricedCalls != 0 {
			t.Errorf("%s: cost line = %+v, want %d µUSD with nothing unpriced", c.name, got, c.wantCost())
		}
	}
}

func TestCallStatsPriceACallAtTheModelThatAnswered(t *testing.T) {
	f := seedAnsweredCalls(t)
	rows, err := NewCallReadStore(f.db).CallStats(diagnosticsReader(f.ws), CallStatsQuery{Window: 24 * time.Hour, GroupBy: GroupByTask})
	if err != nil {
		t.Fatal(err)
	}
	byTask := map[string]CallStatsRow{}
	for _, row := range rows {
		byTask[row.Key] = row
	}
	for i, c := range answeredCases {
		if got := byTask[string(f.tasks[i])]; got.CostMicroUSD != c.wantCost() || got.Unpriced != 0 {
			t.Errorf("%s: figures = %+v, want %d µUSD with nothing unpriced", c.name, got, c.wantCost())
		}
	}
}

func TestARunSummaryPricesACallAtTheModelThatAnswered(t *testing.T) {
	f := seedAnsweredCalls(t)
	reader := principal.WithActor(f.ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + ids.NewV7().String(), UserID: ids.NewV7(),
		Permissions: principal.Permissions{
			RoleKeys: []string{"admin"}, RowScope: principal.RowScopeAll,
			Objects: map[string]principal.ObjectGrant{"company": {Read: true}},
		},
	})
	for i, c := range answeredCases {
		summary, err := NewRunTransparency(f.db).Get(reader, f.correlations[i])
		if err != nil {
			t.Fatal(err)
		}
		if summary.EstimatedCostMicroUSD != c.wantCost() || summary.UnpricedCalls != 0 {
			t.Errorf("%s: run summary = %+v, want %d µUSD with nothing unpriced", c.name, summary, c.wantCost())
		}
	}
}

func TestCallStatsGroupByTheModelThatAnsweredAndCountTheSwitch(t *testing.T) {
	f := seedAnsweredCalls(t)
	reader := NewCallReadStore(f.db)
	rows, err := reader.CallStats(diagnosticsReader(f.ws), CallStatsQuery{Window: 24 * time.Hour, GroupBy: GroupByServedModel, Filter: CallStatsFilter{Tier: string(TierPremium)}})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string][2]int64{}
	for _, row := range rows {
		got[row.Key] = [2]int64{row.Calls, row.Switched}
	}
	want := map[string][2]int64{answeredGemini: {1, 1}, answeredGemini + "-001": {1, 1}, boundGemini: {2, 0}}
	if len(got) != len(want) {
		t.Fatalf("groups = %v, want %v", got, want)
	}
	for key, figures := range want {
		if got[key] != figures {
			t.Errorf("%s: (calls, switched) = %v, want %v", key, got[key], figures)
		}
	}

	tiers, err := reader.CallStats(diagnosticsReader(f.ws), CallStatsQuery{Window: 24 * time.Hour, GroupBy: GroupByTier})
	if err != nil {
		t.Fatal(err)
	}
	if len(tiers) != 1 || tiers[0].Switched != 2 || !strings.HasPrefix(tiers[0].SwitchedTo, answeredGemini) {
		t.Errorf("tier figures = %+v, want 2 of 4 calls switched to %s", tiers, answeredGemini)
	}
}

func TestATaskStepNamesTheModelTheProviderSwitchedTo(t *testing.T) {
	f := seedAnsweredCalls(t)
	reader := NewCallReadStore(f.db)
	for i, c := range answeredCases {
		flow, err := reader.TaskFlow(diagnosticsReader(f.ws), f.tasks[i], 24*time.Hour)
		if err != nil {
			t.Fatal(err)
		}
		wantSwitched, wantTo := int64(0), ""
		if c.servedSource == servedIdentitySourceResponse {
			wantSwitched, wantTo = 1, c.servedModel
		}
		if len(flow.Steps) != 1 || flow.Steps[0].Switched != wantSwitched || flow.Steps[0].SwitchedTo != wantTo {
			t.Errorf("%s: steps = %+v, want one step switched %d to %q", c.name, flow.Steps, wantSwitched, wantTo)
		}
	}
}

func TestTheCallListNarrowsToTheModelThatAnswered(t *testing.T) {
	f := seedAnsweredCalls(t)
	page, err := NewCallReadStore(f.db).ListCalls(diagnosticsReader(f.ws), nil, nil, CallListFilter{ServedModel: answeredGemini})
	if err != nil {
		t.Fatal(err)
	}
	// The echoed call reflected the same id but did not answer as it.
	if len(page.Items) != 1 || page.Items[0].Task != string(f.tasks[0]) {
		t.Fatalf("listed %+v, want only the call the provider answered as %s", page.Items, answeredGemini)
	}
}

func TestAPriceSyncReadsOnlyTheModelsAProviderSwitchedTo(t *testing.T) {
	f := seedAnsweredCalls(t)
	got, err := NewRateStore(f.db).switchedServedModels(f.ctx, time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{answeredGemini, answeredGemini + "-001"}
	if len(got) != 1 || !slices.Equal(got[providerGemini], want) {
		t.Errorf("switched models = %v, want gemini: %v and no echoed or assumed model", got, want)
	}
}
