// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The catalogue refresh against a real sheet: what it writes, what it leaves
// alone, and that pressing it twice is the same as pressing it once. The
// engine takes the broker's answer as a value, so no test here touches a network.

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

const refreshHost = "https://openrouter.ai/api/v1"

func brokerRouting(models ...string) ai.RoutingConfig {
	cfg := ai.RoutingConfig{Tiers: map[ai.Tier]ai.ProviderConfig{}}
	tiers := []ai.Tier{"premium", "cheap_cloud", "local_small"}
	for i, m := range models {
		cfg.Tiers[tiers[i]] = ai.ProviderConfig{Provider: "openai_compatible", Model: m, BaseURL: refreshHost}
	}
	return cfg
}

func listed(id, input, output string, cacheRead *string) ai.AvailableModel {
	return ai.AvailableModel{Info: model.Info{ID: id}, InputPerMtok: &input, OutputPerMtok: &output, CacheReadPerMtok: cacheRead}
}

func catalogueOf(models ...ai.AvailableModel) ai.AvailableModels {
	return ai.AvailableModels{Provider: "openrouter", Models: models}
}

func lineOf(t *testing.T, report ai.RateRefreshReport, provider string) ai.ProviderRefresh {
	t.Helper()
	for _, p := range report.Providers {
		if p.Provider == provider {
			return p
		}
	}
	t.Fatalf("the report has no line for %q: %+v", provider, report)
	return ai.ProviderRefresh{}
}

func seedSheetRate(ctx context.Context, t *testing.T, store *ai.RateStore, provider, modelID, input string, lane ai.Lane, day time.Time) {
	t.Helper()
	if _, err := store.SetModelRate(ctx, ai.SetModelRateInput{
		Provider: provider, ModelID: modelID, InputUsd: input, OutputUsd: "1",
		CacheReadUsd: "0", CacheWriteUsd: "0", Lane: lane, EffectiveDate: day, Source: ai.RateSourceSeed,
	}); err != nil {
		t.Fatalf("seeding %s/%s: %v", provider, modelID, err)
	}
}

func rateAt(ctx context.Context, t *testing.T, store *ai.RateStore, provider, modelID string) ai.ModelRateRow {
	t.Helper()
	rows, err := store.ListEffectiveModelRates(ctx)
	if err != nil {
		t.Fatalf("ListEffectiveModelRates: %v", err)
	}
	for _, r := range rows {
		if r.Provider == provider && r.ModelID == modelID {
			return r
		}
	}
	t.Fatalf("no price in force for %s/%s", provider, modelID)
	return ai.ModelRateRow{}
}

func rateAudits(e *Env, t *testing.T) int {
	t.Helper()
	return e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type='ai_model_rate'`)
}

func TestSyncPricesWritesTheBoundAndSheetModelsOnceOnly(t *testing.T) {
	e := Setup(t)
	today := pinnedRateDay()
	store := ai.NewRateStore(e.DB()).WithClock(func() time.Time { return today })
	ctx := e.Admin()
	seedSheetRate(ctx, t, store, "openai_compatible", "b/sheet-only", "9", ai.LaneEmbeddings, today)
	seedSheetRate(ctx, t, store, "openai_compatible", "c/unlisted", "3", ai.LaneChat, today)
	seedSheetRate(ctx, t, store, "gemini", "gemini-x", "1", ai.LaneChat, today)
	cfg := brokerRouting("a/bound")
	half := "0.5"
	catalogue := catalogueOf(listed("a/bound", "5", "25", &half), listed("b/sheet-only", "1", "2", nil), listed("z/not-ours", "1", "1", nil))

	report, err := store.SyncPrices(ctx, ai.PriceSources{Routing: cfg, Broker: catalogue}, discardRun)
	if err != nil {
		t.Fatalf("first refresh: %v", err)
	}

	line := lineOf(t, report, "openai_compatible")
	if line.Outcome != ai.RefreshUpdated || line.Updated != 1 || line.Added != 1 || len(line.Models) != 2 {
		t.Fatalf("first run line = %+v, want a/bound added and b/sheet-only re-priced", line)
	}
	bound := rateAt(ctx, t, store, "openai_compatible", "a/bound")
	if bound.InputUsd != "5" || bound.OutputUsd != "25" || bound.CacheReadUsd != "0.5" || bound.CacheWriteUsd != "0" || bound.Lane != ai.LaneChat {
		t.Errorf("bound model = %+v, want 5/25 with a 0.5 cache read, a zero cache write, in the chat lane", bound)
	}
	sheetOnly := rateAt(ctx, t, store, "openai_compatible", "b/sheet-only")
	if sheetOnly.InputUsd != "1" || sheetOnly.Lane != ai.LaneEmbeddings {
		t.Errorf("sheet-only model = %+v, want the new price under the lane it was filed as", sheetOnly)
	}
	if unlisted := rateAt(ctx, t, store, "openai_compatible", "c/unlisted"); unlisted.InputUsd != "3" {
		t.Errorf("a model the list no longer names was repriced: %+v", unlisted)
	}
	if other := rateAt(ctx, t, store, "gemini", "gemini-x"); other.InputUsd != "1" {
		t.Errorf("another provider's price moved: %+v", other)
	}
	if got := lineOf(t, report, "gemini").Outcome; got != ai.RefreshNotConfigured {
		t.Errorf("gemini = %q, want not_configured: the harness holds no gemini key", got)
	}

	before := rateAudits(e, t)
	again, err := store.SyncPrices(ctx, ai.PriceSources{Routing: cfg, Broker: catalogue}, discardRun)
	if err != nil {
		t.Fatalf("second refresh: %v", err)
	}
	line = lineOf(t, again, "openai_compatible")
	if line.Outcome != ai.RefreshUnchanged || line.Updated != 0 || line.Unchanged != 2 {
		t.Errorf("second run line = %+v, want both models unchanged", line)
	}
	if after := rateAudits(e, t); after != before {
		t.Errorf("audit rows %d -> %d: a run that changed nothing must leave no trace", before, after)
	}
}

// A price scheduled for tomorrow is somebody's decision, and the refresh writes
// today's row beside it rather than over it.
func TestSyncPricesLeavesAFutureDatedPriceAlone(t *testing.T) {
	e := Setup(t)
	today := pinnedRateDay()
	store := ai.NewRateStore(e.DB()).WithClock(func() time.Time { return today })
	ctx := e.Admin()
	seedSheetRate(ctx, t, store, "openai_compatible", "a/bound", "5", ai.LaneChat, today)
	seedSheetRate(ctx, t, store, "openai_compatible", "a/bound", "7", ai.LaneChat, today.AddDate(0, 0, 1))
	cfg := brokerRouting("a/bound")

	// Equal to the price in force today: nothing to write, though the sheet's
	// head (tomorrow's row) differs from the catalogue.
	if _, err := store.SyncPrices(ctx, ai.PriceSources{Routing: cfg, Broker: catalogueOf(listed("a/bound", "5", "1", nil))}, discardRun); err != nil {
		t.Fatalf("refresh at the price in force: %v", err)
	}
	hist, err := store.ModelRateHistory(ctx, "openai_compatible", "a/bound")
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(hist) != 2 {
		t.Fatalf("history = %+v, want the two rows the sheet already had", hist)
	}

	if _, err := store.SyncPrices(ctx, ai.PriceSources{Routing: cfg, Broker: catalogueOf(listed("a/bound", "6", "1", nil))}, discardRun); err != nil {
		t.Fatalf("refresh at a new price: %v", err)
	}
	if got := rateAt(ctx, t, store, "openai_compatible", "a/bound"); got.InputUsd != "6" {
		t.Errorf("price in force = %+v, want 6", got)
	}
	hist, err = store.ModelRateHistory(ctx, "openai_compatible", "a/bound")
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if len(hist) != 2 || hist[0].InputUsd != "7" {
		t.Errorf("history = %+v, want tomorrow's scheduled 7 still the head, today's row corrected in place", hist)
	}
}

func TestSyncPricesWritesNothingWhenTheListCannotBeRead(t *testing.T) {
	e := Setup(t)
	today := pinnedRateDay()
	store := ai.NewRateStore(e.DB()).WithClock(func() time.Time { return today })
	ctx := e.Admin()
	seedSheetRate(ctx, t, store, "openai_compatible", "a/bound", "5", ai.LaneChat, today)
	before := rateAudits(e, t)

	report, err := store.SyncPrices(ctx, ai.PriceSources{Routing: brokerRouting("a/bound"), Broker: ai.AvailableModels{Provider: "openrouter", Unavailable: ai.AvailabilityUnreachable}}, discardRun)
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}

	line := lineOf(t, report, "openai_compatible")
	if line.Outcome != ai.RefreshUnreachable || line.Updated != 0 {
		t.Errorf("line = %+v, want unreachable with nothing written", line)
	}
	if after := rateAudits(e, t); after != before {
		t.Errorf("audit rows %d -> %d on an unreachable list", before, after)
	}
}

// A decision model the broker prices as variable (-1) stays a manual price.
func TestSyncPricesReportsAModelTheListDoesNotPriceAsNotAvailable(t *testing.T) {
	e := Setup(t)
	store := ai.NewRateStore(e.DB()).WithClock(pinnedRateDay)
	cfg := ai.RoutingConfig{Decisions: &ai.DecisionsConfig{
		Provider: "jev_compatible", Model: "typesafe/jev-router",
		BaseURL: "https://openrouter.ai/api/alpha/decisions",
	}}

	report, err := store.SyncPrices(e.Admin(), ai.PriceSources{Routing: cfg, Broker: catalogueOf(ai.AvailableModel{Info: model.Info{ID: "typesafe/jev-router"}})}, discardRun)
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}

	if got := lineOf(t, report, "jev_compatible"); got.Outcome != ai.RefreshNotAvailable || got.Updated != 0 {
		t.Errorf("jev_compatible = %+v, want not_available", got)
	}
	if got := lineOf(t, report, "openai_compatible").Outcome; got != ai.RefreshNotBound {
		t.Errorf("openai_compatible = %q, want not_bound when nothing is", got)
	}
}

// One transaction: a principal who may insert but not overwrite is refused at
// the overwrite, and the insert that came before it in the same run rolls back.
func TestSyncPricesIsAllOrNothingAndHonoursTheWriteGrants(t *testing.T) {
	e := Setup(t)
	today := pinnedRateDay()
	store := ai.NewRateStore(e.DB()).WithClock(func() time.Time { return today })
	seedSheetRate(e.Admin(), t, store, "openai_compatible", "b/sheet-only", "9", ai.LaneChat, today)
	creator := e.As(e.Rep1, nil, modelRatePerms(principal.ObjectGrant{Create: true, Read: true}))
	before := rateAudits(e, t)

	_, err := store.SyncPrices(creator, ai.PriceSources{Routing: brokerRouting("a/bound"), Broker: catalogueOf(listed("a/bound", "5", "25", nil), listed("b/sheet-only", "1", "2", nil))}, discardRun)

	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("create-only refresh = %v, want ErrPermissionDenied", err)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM ai_model_rate WHERE provider='openai_compatible' AND model_id='a/bound'`); n != 0 {
		t.Errorf("the insert that preceded the refused overwrite survived: %d row(s)", n)
	}
	if after := rateAudits(e, t); after != before {
		t.Errorf("audit rows %d -> %d after a refused run", before, after)
	}
}

func TestSyncPricesRefusesAPrincipalWithoutTheSheet(t *testing.T) {
	e := Setup(t)
	store := ai.NewRateStore(e.DB())
	reader := e.As(e.Rep1, nil, ReadOnlyPerms)

	_, err := store.SyncPrices(reader, ai.PriceSources{Routing: brokerRouting("a/bound"), Broker: catalogueOf(listed("a/bound", "5", "25", nil))}, discardRun)

	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("read-only refresh = %v, want ErrPermissionDenied", err)
	}
}

// The route is human-only in the contract, the generated policy table and the
// handler. This drives the first two: an agent's bearer is refused before the
// handler runs, so the refusal cannot depend on the handler being wired.
func TestRefreshAiModelRatesRefusesAnAgentBearerOverHTTP(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	var minted struct {
		Token string `json:"token"`
	}
	if status := e.Call(t, "POST", "/v1/passports", AnyMap{
		"label": "refresh probe", "scopes": []string{"read"},
	}, nil, &minted); status != http.StatusCreated {
		t.Fatalf("issue passport → %d", status)
	}
	bearer := map[string]string{"Authorization": "Bearer " + minted.Token}

	var problem struct {
		Code string `json:"code"`
	}
	status := e.Call(t, "POST", "/v1/ai-model-rates/refresh", nil, bearer, &problem)

	if status != http.StatusForbidden || problem.Code != "permission_denied" {
		t.Errorf("agent POST /ai-model-rates/refresh → %d %q, want 403 permission_denied", status, problem.Code)
	}
}

// A misspelt bound id is neither priced nor "set by hand": the list simply does
// not name it, and the report must say so and carry the id.
func TestSyncPricesNamesABoundModelTheListDoesNotHave(t *testing.T) {
	e := Setup(t)
	store := ai.NewRateStore(e.DB()).WithClock(pinnedRateDay)

	report, err := store.SyncPrices(e.Admin(), ai.PriceSources{Routing: brokerRouting("a/typo", "b/real"), Broker: catalogueOf(listed("b/real", "1", "2", nil))}, discardRun)
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}

	line := lineOf(t, report, "openai_compatible")
	if len(line.Unlisted) != 1 || line.Unlisted[0] != "a/typo" {
		t.Fatalf("line = %+v, want a/typo unlisted", line)
	}
	if line.Outcome != ai.RefreshUpdated {
		t.Errorf("outcome = %q: a run that wrote b/real is updated, with the typo listed beside it", line.Outcome)
	}

	only, err := store.SyncPrices(e.Admin(), ai.PriceSources{Routing: brokerRouting("a/typo"), Broker: catalogueOf(listed("z/else", "1", "2", nil))}, discardRun)
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if got := lineOf(t, only, "openai_compatible").Outcome; got != ai.RefreshNotListed {
		t.Errorf("outcome = %q, want not_listed when nothing bound is named", got)
	}
}

// Rows on the sheet under a host the refresh does not serve stay as typed.
func TestSyncPricesLeavesASelfHostedSheetRowAlone(t *testing.T) {
	e := Setup(t)
	today := pinnedRateDay()
	store := ai.NewRateStore(e.DB()).WithClock(func() time.Time { return today })
	seedSheetRate(e.Admin(), t, store, "openai_compatible", "meta/llama-4", "0", ai.LaneChat, today)
	selfHosted := ai.RoutingConfig{Tiers: map[ai.Tier]ai.ProviderConfig{
		"premium": {Provider: "openai_compatible", Model: "meta/llama-4", BaseURL: "https://llm.internal.test/v1"},
	}}

	if _, err := store.SyncPrices(e.Admin(), ai.PriceSources{Routing: selfHosted, Broker: catalogueOf(listed("meta/llama-4", "5", "25", nil))}, discardRun); err != nil {
		t.Fatalf("refresh: %v", err)
	}

	if got := rateAt(e.Admin(), t, store, "openai_compatible", "meta/llama-4"); got.InputUsd != "0" {
		t.Errorf("a self-hosted model priced 0 by hand became %+v", got)
	}
}

func discardRun(context.Context, pgx.Tx, ai.RateRefreshReport) error { return nil }
