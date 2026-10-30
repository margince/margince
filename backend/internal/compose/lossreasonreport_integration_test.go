// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/analyticsquery"
	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// lossAdmin may write deals, decide mappings and run a correction. The task
// rebuilds its operator from the stored role, so the role holds the grant too.
func lossAdmin(t *testing.T, e *integration.Env) context.Context {
	t.Helper()
	e.GrantRole(t, e.Rep1, "admin")
	e.WsExec(t, `UPDATE role SET permissions = jsonb_set(permissions, '{objects,data_task}',
		'{"read":true,"update":true,"create":false,"delete":false}'::jsonb) WHERE key = 'admin'`)
	objects := maps.Clone(integration.AdminPerms.Objects)
	objects["custom_field"] = principal.ObjectGrant{Create: true, Read: true, Update: true}
	objects["data_task"] = principal.ObjectGrant{Read: true, Update: true}
	return e.As(e.Rep1, []ids.UUID{e.Team1}, principal.Permissions{
		RoleKeys: integration.AdminPerms.RoleKeys, Objects: objects, RowScope: principal.RowScopeAll,
	})
}

// seedLosses loses four EUR deals through the deal store. One has a code the
// closer chose, two share a legacy note, and one has a note nobody maps.
func seedLosses(ctx context.Context, t *testing.T, e *integration.Env) {
	t.Helper()
	pipeline, open, _ := integration.DealFixture(t, e)
	p, err := e.Deals.DefaultPipeline(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var lost ids.StageID
	for _, st := range *p.Stages {
		if st.Semantic == "lost" {
			lost = ids.From[ids.StageKind](ids.UUID(st.Id))
		}
	}
	eur := "EUR"
	for i, loss := range []struct {
		amount     int64
		note, code *string
	}{
		{1000, nil, new("price")},
		{2000, new("Their quote was cheaper"), nil},
		{3000, new("their quote was cheaper "), nil},
		{4000, new("The champion left"), nil},
	} {
		amount := loss.amount
		made, err := e.Deals.CreateDeal(ctx, deals.CreateDealInput{
			Name: fmt.Sprintf("Loss %d", i), PipelineID: pipeline, StageID: open, AmountMinor: &amount, Currency: &eur,
		})
		if err != nil {
			t.Fatalf("creating loss %d: %v", i, err)
		}
		if _, err := e.Deals.AdvanceDeal(ctx, ids.From[ids.DealKind](ids.UUID(made.Id)), deals.AdvanceDealInput{
			ToStageID: lost, LostReason: loss.note, LostReasonCode: loss.code,
		}); err != nil {
			t.Fatalf("losing %d: %v", i, err)
		}
	}
}

// decideShared decides the note two deals share as price.
func decideShared(ctx context.Context, t *testing.T, e *integration.Env) crmcontracts.LossReasonMapping {
	t.Helper()
	listed, err := e.Deals.ListLegacyLossReasons(ctx, deals.MaxLegacyLossReasons)
	if err != nil {
		t.Fatal(err)
	}
	for _, note := range listed.Data {
		if note.DealCount == 2 {
			decided, err := e.Deals.DecideLegacyLossReason(ctx, note.TextHash, "price")
			if err != nil {
				t.Fatal(err)
			}
			return decided
		}
	}
	t.Fatalf("no note is shared by two deals: %+v", listed.Data)
	return crmcontracts.LossReasonMapping{}
}

// lossTaskRunner applies reviewed decisions as an administrator does. The
// HTTP door activates the reviewed run, and the data task engine carries it out.
type lossTaskRunner struct {
	e      *integration.Env
	engine *datatask.Engine
	system context.Context
	now    func() time.Time
}

func newLossTaskRunner(t *testing.T, e *integration.Env) lossTaskRunner {
	t.Helper()
	now := func() time.Time { return clocktest.Now(t) }
	engine, err := datatask.New(e.Pool, []datatask.Task{lossMappingDataTask(e.Pool)}, "test", now,
		func(context.Context, pgx.Tx, datatask.Progress) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	system := principal.SystemActing(e.Admin(), "system:loss-mapping-test")
	if err := engine.Seed(system); err != nil {
		t.Fatal(err)
	}
	return lossTaskRunner{e: e, engine: engine, system: system, now: now}
}

// apply posts the decisions shown, runs the task to its end, and answers the
// HTTP status and the deals the run said it would change.
func (r lossTaskRunner) apply(ctx context.Context, t *testing.T, shown ...crmcontracts.LossReasonMapping) (int, int) {
	t.Helper()
	mappings := make([]map[string]any, 0, len(shown))
	for _, mapping := range shown {
		mappings = append(mappings, map[string]any{"id": mapping.Id, "version": mapping.Version})
	}
	raw, err := json.Marshal(map[string]any{"mappings": mappings})
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	(dataTaskHandlers{pool: r.e.Pool, clock: r.now}).ApplyLegacyLossReasons(rec,
		httptest.NewRequest(http.MethodPost, "/v1/loss-reasons/legacy/apply", bytes.NewReader(raw)).WithContext(ctx))
	if rec.Code != http.StatusAccepted {
		return rec.Code, 0
	}
	var answer crmcontracts.ApplyLegacyLossReasonsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &answer); err != nil {
		t.Fatal(err)
	}
	for range 5 {
		if _, err := r.engine.Execute(r.system); err != nil {
			t.Fatalf("running the mapping task: %v", err)
		}
	}
	return rec.Code, answer.Deals
}

func askWinLoss(t *testing.T, e *integration.Env, groupBy ...string) map[string]string {
	t.Helper()
	ctx := e.Admin()
	var answer AnalyticsAnswer
	q := analyticsquery.Query{
		Entity: "win-loss", GroupBy: append(groupBy, "currency"),
		Measures: []analyticsquery.Measure{{Fn: analyticsquery.CountAll}, {Fn: analyticsquery.Sum, Field: "amount_minor", As: "amount"}},
	}
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		var err error
		answer, err = RunAnalyticsQuery(ctx, tx, q, analyticsquery.Floor(1))
		return err
	}); err != nil {
		t.Fatalf("asking win-loss by %v: %v", groupBy, err)
	}
	out := map[string]string{}
	for _, row := range answer.Rows {
		amount, err := json.Marshal(row["amount"])
		if err != nil {
			t.Fatal(err)
		}
		out[fmt.Sprint(row[groupBy[0]])] = fmt.Sprintf("%v/%s", row["count"], amount)
	}
	return out
}

// The loss reason dimensions split the same losses the status dimension
// counts, money included. A note nobody mapped stays its own bucket rather
// than vanishing or borrowing a code.
func TestWinLossCountsLossesByHowTheirReasonIsKnown(t *testing.T) {
	e := integration.Setup(t)
	operator := lossAdmin(t, e)
	seedLosses(operator, t, e)
	if status, _ := newLossTaskRunner(t, e).apply(operator, t, decideShared(operator, t, e)); status != http.StatusAccepted {
		t.Fatalf("apply answered %d", status)
	}

	if got, want := askWinLoss(t, e, "status"), map[string]string{"lost": "4/10000"}; !maps.Equal(got, want) {
		t.Fatalf("losses by status = %v, want %v", got, want)
	}
	states := askWinLoss(t, e, "loss_reason_state")
	if want := map[string]string{"recorded": "1/1000", "mapped": "2/5000", "unmapped": "1/4000"}; !maps.Equal(states, want) {
		t.Errorf("losses by how the reason is known = %v, want %v", states, want)
	}
	reasons := askWinLoss(t, e, "loss_reason")
	if want := map[string]string{"price": "3/6000", "<nil>": "1/4000"}; !maps.Equal(reasons, want) {
		t.Errorf("losses by reason = %v, want %v", reasons, want)
	}
}

// One run changes at most a page of deals. The deals it changed leave the
// pending set, so applying the same decision again takes the rest.
func TestAnApplyLargerThanOneRunFinishesOnTheNext(t *testing.T) {
	e := integration.Setup(t)
	operator := lossAdmin(t, e)
	seedLosses(operator, t, e)
	page := maxReviewedLossDeals
	maxReviewedLossDeals = 1
	t.Cleanup(func() { maxReviewedLossDeals = page })
	runner := newLossTaskRunner(t, e)
	decided := decideShared(operator, t, e)
	for run, want := range []int{1, 1} {
		if status, deals := runner.apply(operator, t, decided); status != http.StatusAccepted || deals != want {
			t.Fatalf("run %d answered %d for %d deals, want %d", run+1, status, deals, want)
		}
	}
	if status, deals := runner.apply(operator, t, decided); status != http.StatusAccepted || deals != 0 {
		t.Errorf("a third apply answered %d for %d deals, want nothing left", status, deals)
	}
	if got := askWinLoss(t, e, "loss_reason_state")["mapped"]; got != "2/5000" {
		t.Errorf("mapped losses = %q, want both deals the note covers", got)
	}
}

// An apply carries the decisions the administrator saw, at the versions they
// saw. One that moved since is refused, and one nobody named is not applied.
func TestAnApplyCarriesOnlyTheDecisionsItWasShown(t *testing.T) {
	e := integration.Setup(t)
	operator := lossAdmin(t, e)
	seedLosses(operator, t, e)
	runner := newLossTaskRunner(t, e)
	shared := decideShared(operator, t, e)
	listed, err := e.Deals.ListLegacyLossReasons(operator, deals.MaxLegacyLossReasons)
	if err != nil {
		t.Fatal(err)
	}
	var unseen crmcontracts.LossReasonMapping
	for _, note := range listed.Data {
		if note.Mapping == nil {
			if unseen, err = e.Deals.DecideLegacyLossReason(operator, note.TextHash, "no_decision"); err != nil {
				t.Fatal(err)
			}
		}
	}
	moved, err := e.Deals.DecideLegacyLossReason(operator, shared.TextHash, "competitor")
	if err != nil {
		t.Fatal(err)
	}
	if status, _ := runner.apply(operator, t, shared); status != http.StatusConflict {
		t.Fatalf("an apply of a decision that moved answered %d, want 409", status)
	}
	if status, deals := runner.apply(operator, t, moved); status != http.StatusAccepted || deals != 2 {
		t.Fatalf("an apply of the decision as it stands answered %d for %d deals", status, deals)
	}
	reasons := askWinLoss(t, e, "loss_reason")
	if reasons["competitor"] != "2/5000" || reasons["no_decision"] != "" {
		t.Errorf("losses by reason = %v: the shown decision applies, the unseen %s does not", reasons, unseen.ReasonKey)
	}
}

// A deal lost, reopened and lost again counts once in win-loss, under its last
// loss's reason, which is what the report's notes state. The first loss keeps
// its own code on its stage history row.
func TestAReloseCountsOnceUnderItsLastLoss(t *testing.T) {
	e := integration.Setup(t)
	ctx := lossAdmin(t, e)
	pipeline, open, _ := integration.DealFixture(t, e)
	p, err := e.Deals.DefaultPipeline(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var lost ids.StageID
	for _, st := range *p.Stages {
		if st.Semantic == "lost" {
			lost = ids.From[ids.StageKind](ids.UUID(st.Id))
		}
	}
	amount, eur := int64(7000), "EUR"
	made, err := e.Deals.CreateDeal(ctx, deals.CreateDealInput{Name: "Lost twice", PipelineID: pipeline, StageID: open, AmountMinor: &amount, Currency: &eur})
	if err != nil {
		t.Fatal(err)
	}
	deal := ids.From[ids.DealKind](ids.UUID(made.Id))
	for _, move := range []deals.AdvanceDealInput{
		{ToStageID: lost, LostReasonCode: new("price")},
		{ToStageID: open},
		{ToStageID: lost, LostReasonCode: new("competitor")},
	} {
		if _, err := e.Deals.AdvanceDeal(ctx, deal, move); err != nil {
			t.Fatal(err)
		}
	}
	if got, want := askWinLoss(t, e, "loss_reason"), map[string]string{"competitor": "1/7000"}; !maps.Equal(got, want) {
		t.Errorf("losses by reason = %v, want %v", got, want)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM deal_stage_history
		WHERE lost_reason_code_at_change IN ('price', 'competitor')`); got != 2 {
		t.Errorf("the stage history holds %d coded losses, want both", got)
	}
}
