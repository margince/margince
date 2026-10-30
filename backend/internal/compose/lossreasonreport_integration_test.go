// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
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

// applyThroughTheTask decides the shared note as price and applies it as an
// administrator does. The HTTP door activates the reviewed run, and the data
// task engine carries it out.
func applyThroughTheTask(ctx context.Context, t *testing.T, e *integration.Env) {
	t.Helper()
	listed, err := e.Deals.ListLegacyLossReasons(ctx, deals.MaxLegacyLossReasons)
	if err != nil {
		t.Fatal(err)
	}
	for _, note := range listed.Data {
		if note.DealCount == 2 {
			if _, err := e.Deals.DecideLegacyLossReason(ctx, note.TextHash, "price"); err != nil {
				t.Fatal(err)
			}
		}
	}
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
	rec := httptest.NewRecorder()
	(dataTaskHandlers{pool: e.Pool, clock: now}).ApplyLegacyLossReasons(rec,
		httptest.NewRequest(http.MethodPost, "/v1/loss-reasons/legacy/apply", nil).WithContext(ctx))
	if rec.Code != http.StatusAccepted {
		t.Fatalf("apply answered %d %s", rec.Code, rec.Body.String())
	}
	for range 5 {
		if _, err := engine.Execute(system); err != nil {
			t.Fatalf("running the mapping task: %v", err)
		}
	}
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
	applyThroughTheTask(operator, t, e)

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
