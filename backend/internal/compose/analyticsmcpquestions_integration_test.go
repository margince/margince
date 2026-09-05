// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The canonical persona questions, answered through the MCP tool runner
// against a real database. testdata/analytics_mcp_questions.json is the one
// spelling of each question; the census below holds the file and these cases
// equal in both directions, so a question cannot be added without proof or
// proved under a wording the corpus no longer carries.

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"

	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

type canonicalQuestion struct {
	ID       string `json:"id"`
	Persona  string `json:"persona"`
	Question string `json:"question"`
	Tool     string `json:"tool"`
	Expects  string `json:"expects"`
}

func loadCanonicalQuestions(t *testing.T) []canonicalQuestion {
	t.Helper()
	raw, err := os.ReadFile("testdata/analytics_mcp_questions.json")
	if err != nil {
		t.Fatalf("reading the canonical corpus: %v", err)
	}
	var doc struct {
		Questions []canonicalQuestion `json:"questions"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("the canonical corpus is not the shape this suite reads: %v", err)
	}
	if len(doc.Questions) == 0 {
		t.Fatal("the corpus holds no questions — the census below would compare two empty sets")
	}
	return doc.Questions
}

// canonicalCases maps each corpus id to its executable proof.
func canonicalCases() map[string]func(t *testing.T) {
	return map[string]func(t *testing.T){
		"G01": proveG01OwnPipeline,
		"G02": proveG02WorkspaceRefused,
		"G03": proveG03OneBaseCurrencyNumber,
		"M04": proveM04OneRowPerStage,
		"M06": proveM06MedianUnderTheFloor,
	}
}

func TestEveryCanonicalQuestionHasExecutableProof(t *testing.T) {
	cases := canonicalCases()
	seen := map[string]bool{}
	for _, q := range loadCanonicalQuestions(t) {
		if q.Tool != "run_analytics_query" {
			t.Errorf("%s names tool %q, which this suite does not drive — either the corpus "+
				"or the case map is ahead of the other", q.ID, q.Tool)
		}
		if _, proved := cases[q.ID]; !proved {
			t.Errorf("%s (%q) has no executable case — a question without proof is prose", q.ID, q.Question)
		}
		if seen[q.ID] {
			t.Errorf("%s appears twice in the corpus; the second wording is silently unproved", q.ID)
		}
		seen[q.ID] = true
	}
	for id := range cases {
		if !seen[id] {
			t.Errorf("case %s proves a question the corpus no longer carries", id)
		}
	}
}

func TestCanonicalAnalyticsQuestions(t *testing.T) {
	// In the corpus's own order, so a failing run reads the way the canonical
	// file does and two invocations report in one order.
	cases := canonicalCases()
	for _, q := range loadCanonicalQuestions(t) {
		if prove, ok := cases[q.ID]; ok {
			t.Run(q.ID, prove)
		}
	}
}

// askTool drives the real MCP runner and decodes its answer.
func askTool(ctx context.Context, t *testing.T, e *forecastEnv, args string) (agents.AnalyticsQueryResult, error) {
	t.Helper()
	raw, err := analyticsQueryToolRunner(InstallationDB(e.Pool))(ctx, json.RawMessage(args))
	if err != nil {
		return agents.AnalyticsQueryResult{}, err
	}
	var out agents.AnalyticsQueryResult
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("the tool's answer is not its own declared shape: %v", err)
	}
	return out, nil
}

func toolRow(t *testing.T, raw json.RawMessage) map[string]any {
	t.Helper()
	var row map[string]any
	if err := json.Unmarshal(raw, &row); err != nil {
		t.Fatalf("a row is not an object: %v", err)
	}
	return row
}

// G01: How much pipeline do I have right now? An omitted scope is the rep's
// own deals. The scope RULE is owned by the typed suite next door
// (TestATypedQueryAnswersTheAskersOwnPopulation); what this case adds is the
// transport — the rule surviving the tool runner's decode-and-encode path.
func proveG01OwnPipeline(t *testing.T) {
	e := setupForecast(t)
	amount := int64(100_000)
	for range 6 {
		e.seedOpenDeal(t, "Mine", 20, &e.Rep1, &amount, nil)
		e.seedOpenDeal(t, "Theirs", 20, &e.Rep3, &amount, nil)
	}
	got, err := askTool(e.ownLensRepCtx(e.Rep1), t, e,
		`{"entity":"pipeline-current","measures":[{"fn":"count","as":"deals"}]}`)
	if err != nil {
		t.Fatalf("a rep's own question was refused: %v", err)
	}
	if len(got.Rows) != 1 {
		t.Fatalf("an ungrouped question answered %d rows", len(got.Rows))
	}
	if n := toolRow(t, got.Rows[0])["deals"]; n != float64(6) {
		t.Errorf("the rep was answered %v deals, want their own 6 of the 12 seeded", n)
	}
}

// G02: Show me the whole company's pipeline by owner. Refused, not narrowed:
// a caller who asserts a wider population and is quietly given a smaller one
// cannot tell the answer from a genuinely small pipeline.
func proveG02WorkspaceRefused(t *testing.T) {
	e := setupForecast(t)
	amount := int64(100_000)
	for range 6 {
		e.seedOpenDeal(t, "Somebody's", 20, &e.Rep3, &amount, nil)
	}
	// The admit arm first: the SAME question without the workspace assertion
	// answers, so the refusal below cannot be a validation failure wearing the
	// gate's clothes.
	if _, err := askTool(e.ownLensRepCtx(e.Rep1), t, e,
		`{"entity":"pipeline-current","measures":[{"fn":"count","as":"deals"}]}`); err != nil {
		t.Fatalf("the rep's own question was refused, so the workspace refusal below proves nothing: %v", err)
	}
	_, err := askTool(e.ownLensRepCtx(e.Rep1), t, e,
		`{"entity":"pipeline-current","measures":[{"fn":"count","as":"deals"}],"scope_kind":"workspace"}`)
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("a rep asking for the workspace got %v, want the permission sentinel — "+
			"a 422 here would be the compiler refusing the words, not the gate refusing the population", err)
	}
}

// G03: Add our EUR, USD, and VND pipeline and give me one number. Per-deal
// conversion; the deal whose currency has no rate is excluded from the money
// and still counted — a guessed rate of 1 would invent money.
func proveG03OneBaseCurrencyNumber(t *testing.T) {
	e := setupForecast(t)
	seedRate(t, e, "0.5", 1)
	for _, d := range []struct {
		name, currency string
		amount         int64
	}{
		{"EUR one", "EUR", 10_000},
		{"EUR two", "EUR", 10_000},
		{"EUR three", "EUR", 10_000},
		{"USD one", "USD", 40_000},
		{"USD two", "USD", 40_000},
		{"VND unrated", "VND", 9_000_000},
	} {
		seedPricedDeal(t, e, d.name, d.amount, d.currency, "open")
	}
	got, err := askTool(e.Admin(), t, e, `{"entity":"pipeline-current","measures":[
		{"fn":"count","as":"deals"},
		{"fn":"sum","field":"amount_base_minor","as":"base_total"}]}`)
	if err != nil {
		t.Fatal(err)
	}
	row := toolRow(t, got.Rows[0])
	if row["deals"] != float64(6) {
		t.Errorf("counted %v deals, want all 6 including the unrated one", row["deals"])
	}
	// 3×10,000 EUR native + 2×40,000 USD at 0.5 = 30,000 + 40,000. The
	// unrated VND deal's nine million minor are NOT in it: excluded, never
	// guessed at rate 1 — and the count above still says six.
	if row["base_total"] != float64(70_000) {
		t.Errorf("base total = %v, want 70000 exactly — per-deal conversion, no native-minor addition", row["base_total"])
	}
}

// M04: How much open pipeline does my team have in each stage right now?
// One row per stage, open deals only.
func proveM04OneRowPerStage(t *testing.T) {
	e := setupForecast(t)
	amount := int64(100_000)
	for range 5 {
		e.seedOpenDeal(t, "Early", 20, &e.Rep1, &amount, nil)
		e.seedOpenDeal(t, "Late", 60, &e.Rep1, &amount, nil)
		// Somebody else's pipeline, which "my" must exclude.
		e.seedOpenDeal(t, "Not mine", 20, &e.Rep3, &amount, nil)
	}
	// Closed deals in the same stages, so "open only" has something to leak.
	seedPricedDeal(t, e, "Won already", 90_000, "EUR", "won")
	seedPricedDeal(t, e, "Lost", 70_000, "EUR", "lost")

	got, err := askTool(e.ownLensRepCtx(e.Rep1), t, e,
		`{"entity":"pipeline-current","group_by":["stage_id"],"measures":[{"fn":"count","as":"deals"}]}`)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Rows) != 2 {
		t.Fatalf("two seeded stages answered %d rows", len(got.Rows))
	}
	for _, raw := range got.Rows {
		if n := toolRow(t, raw)["deals"]; n != float64(5) {
			t.Errorf("a stage row counts %v, want the caller's own 5 open deals — neither "+
				"the closed ones nor a colleague's", n)
		}
	}
}

// M06: Which stages are taking longer than normal? The statistic under it —
// a percentile — answers null below the five-value sample floor, beside an
// honest count: four values do not make a median, and Postgres computing one
// anyway is exactly why the refusal is rendered into the SQL.
func proveM06MedianUnderTheFloor(t *testing.T) {
	e := setupForecast(t)
	amount := int64(100_000)
	for i := range 6 {
		priced := &amount
		if i >= 4 {
			priced = nil // two unpriced deals: six rows, four values
		}
		e.seedOpenDeal(t, "Deal", 20, &e.Rep1, priced, nil)
	}
	got, err := askTool(e.ownLensRepCtx(e.Rep1), t, e, `{"entity":"pipeline-current","measures":[
		{"fn":"count","as":"deals"},{"fn":"median","field":"amount_base_minor","as":"typical"}]}`)
	if err != nil {
		t.Fatal(err)
	}
	row := toolRow(t, got.Rows[0])
	if row["deals"] != float64(6) {
		t.Errorf("counted %v deals, want 6", row["deals"])
	}
	if row["typical"] != nil {
		t.Errorf("a median over four values answered %v, want null beside the count", row["typical"])
	}
}
