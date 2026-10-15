// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/platform/config"
)

// envBinding binds every tier and the embeddings lane to the offline fake
// model, spelled as the environment carries it.
func envBinding(model string) string {
	lane := `{"provider":"fake","model":"` + model + `"}`
	return `MARGINCE_AI_ROUTING_BINDING={"profile":"eu_hosted","tiers":{"local_small":` + lane +
		`,"cheap_cloud":` + lane + `,"premium":` + lane + `,"frontier":` + lane + `},` +
		`"embeddings":{"provider":"fake","model":"` + model + `-embed","dimensions":8}}`
}

// The binding the api's environment sets is the one GET answers. A changed
// binding or provider entry is refused, and the binding read back saves as a
// no-op.
func TestAIRoutingSetByEnvIsLockedAgainstAChange(t *testing.T) {
	e := apptest.SetupAppWithEnv(t, []string{envBinding("env-model")})
	e.BootstrapWorkspace(t)

	var read json.RawMessage
	if status := e.Call(t, http.MethodGet, "/v1/ai/routing", nil, nil, &read); status != http.StatusOK ||
		!strings.Contains(string(read), `"env-model"`) {
		t.Fatalf("GET /ai/routing → %d %s, want the binding the environment sets", status, read)
	}
	changed := strings.ReplaceAll(string(read), `"env-model"`, `"other-model"`)
	if status := e.Call(t, http.MethodPut, "/v1/ai/routing", json.RawMessage(changed), nil, nil); status != http.StatusConflict {
		t.Fatalf("a change of the locked binding → %d, want 409", status)
	}
	if status := e.Call(t, http.MethodPut, "/v1/ai/provider-settings/openai_compatible",
		map[string]string{"base_url": "https://openrouter.ai/api"}, nil, nil); status != http.StatusConflict {
		t.Fatalf("a provider entry added to the locked binding → %d, want 409", status)
	}
	if status := e.Call(t, http.MethodPut, "/v1/ai/routing", read, nil, nil); status != http.StatusOK {
		t.Fatalf("resending the locked binding → %d, want 200", status)
	}
	if rows := settingRows(t, e, ai.RoutingKey); rows != 0 {
		t.Fatalf("the locked binding wrote %d rows, want none", rows)
	}
}

// A boot binding in the per-lane shape, which names a host on each lane, reads
// back lifted onto its provider. That reading resends as a no-op, though it is
// not the decoded boot value field for field.
func TestABootBindingInThePerLaneShapeResendsAsANoOp(t *testing.T) {
	lane := func(model string) string {
		return `{"provider":"openai_compatible","model":"` + model + `","base_url":"https://openrouter.ai/api"}`
	}
	e := apptest.SetupAppWithEnv(t, []string{`MARGINCE_AI_ROUTING_BINDING={"profile":"eu_hosted","tiers":{` +
		`"cheap_cloud":` + lane("mistralai/ministral-8b-2512") + `,"premium":` + lane("mistralai/mistral-small-2603") + `},` +
		`"embeddings":{"provider":"openai_compatible","model":"mistralai/mistral-embed-2312","base_url":"https://openrouter.ai/api","dimensions":1024}}`})
	e.BootstrapWorkspace(t)

	var read json.RawMessage
	if status := e.Call(t, http.MethodGet, "/v1/ai/routing", nil, nil, &read); status != http.StatusOK {
		t.Fatalf("GET /ai/routing → %d %s", status, read)
	}
	if status := e.Call(t, http.MethodPut, "/v1/ai/routing", read, nil, nil); status != http.StatusOK {
		t.Fatalf("resending the lifted boot binding %s → %d, want 200", read, status)
	}
}

// The environment may spell a task reset to the product's values as {}, which
// a save drops; the overrides read back still resend as a no-op.
func TestAITaskOverridesSetByEnvAreLockedAgainstAChange(t *testing.T) {
	e := apptest.SetupAppWithEnv(t, []string{
		`MARGINCE_AI_TASK_OVERRIDES={"capture_classify":{"thinking":"high"},"cold_start":{}}`,
	})
	e.BootstrapWorkspace(t)

	var read json.RawMessage
	if status := e.Call(t, http.MethodGet, "/v1/ai/task-overrides", nil, nil, &read); status != http.StatusOK ||
		!strings.Contains(string(read), `"high"`) {
		t.Fatalf("GET /ai/task-overrides → %d %s, want the high the environment sets", status, read)
	}
	if status := e.Call(t, http.MethodPut, "/v1/ai/task-overrides",
		map[string]any{"capture_classify": map[string]string{"thinking": "low"}}, nil, nil); status != http.StatusConflict {
		t.Fatalf("a change of the locked overrides → %d, want 409", status)
	}
	if status := e.Call(t, http.MethodPut, "/v1/ai/task-overrides", read, nil, nil); status != http.StatusOK {
		t.Fatalf("resending the locked overrides %s → %d, want 200", read, status)
	}
	if status := e.Call(t, http.MethodPut, "/v1/ai/task-overrides",
		map[string]any{"capture_classify": map[string]string{"thinking": "high"}}, nil, nil); status != http.StatusOK {
		t.Fatalf("resending the locked overrides without the reset task → %d, want 200", status)
	}
	if rows := settingRows(t, e, ai.TaskOverridesKey); rows != 0 {
		t.Fatalf("the locked overrides wrote %d rows, want none", rows)
	}
}

func TestAIBudgetSetByEnvIsLockedAgainstAChange(t *testing.T) {
	e := apptest.SetupAppWithEnv(t, []string{
		`MARGINCE_AI_BUDGET={"tokens_per_full_user":5000,"company_monthly_tokens":777}`,
	})
	e.BootstrapWorkspace(t)

	type budgetConfig struct {
		TokensPerFullUser    int64  `json:"tokens_per_full_user"`
		CompanyMonthlyTokens *int64 `json:"company_monthly_tokens"`
	}
	var read struct {
		Config   budgetConfig `json:"config"`
		Revision string       `json:"revision"`
	}
	if status := e.Call(t, http.MethodGet, "/v1/ai/budget", nil, nil, &read); status != http.StatusOK ||
		read.Config.TokensPerFullUser != 5000 || read.Config.CompanyMonthlyTokens == nil || *read.Config.CompanyMonthlyTokens != 777 {
		t.Fatalf("GET /ai/budget → %d %+v, want the allowance the environment sets", status, read.Config)
	}
	more := int64(778)
	changed := budgetConfig{TokensPerFullUser: 5000, CompanyMonthlyTokens: &more}
	if status := e.Call(t, http.MethodPut, "/v1/ai/budget",
		map[string]any{"config": changed, "expected_revision": read.Revision}, nil, nil); status != http.StatusConflict {
		t.Fatalf("a change of the locked allowance → %d, want 409", status)
	}
	if status := e.Call(t, http.MethodPut, "/v1/ai/budget",
		map[string]any{"config": read.Config, "expected_revision": read.Revision}, nil, nil); status != http.StatusOK {
		t.Fatalf("resending the locked allowance → %d, want 200", status)
	}
	if rows := settingRows(t, e, ai.BudgetKey); rows != 0 {
		t.Fatalf("the locked allowance wrote %d rows, want none", rows)
	}
}

func TestAIPriceSyncSetByEnvIsLockedAgainstAChange(t *testing.T) {
	e := apptest.SetupAppWithEnv(t, []string{`MARGINCE_AI_PRICE_SYNC={"auto_sync":false}`})
	e.BootstrapWorkspace(t)

	var read struct {
		AutoSync bool `json:"auto_sync"`
	}
	if status := e.Call(t, http.MethodGet, "/v1/ai/price-sync", nil, nil, &read); status != http.StatusOK || read.AutoSync {
		t.Fatalf("GET /ai/price-sync → %d, auto_sync %v, want the false the environment sets", status, read.AutoSync)
	}
	if status := e.Call(t, http.MethodPut, "/v1/ai/price-sync", map[string]bool{"auto_sync": true}, nil, nil); status != http.StatusConflict {
		t.Fatalf("a change of the locked switch → %d, want 409", status)
	}
	if status := e.Call(t, http.MethodPut, "/v1/ai/price-sync", map[string]bool{"auto_sync": false}, nil, nil); status != http.StatusOK {
		t.Fatalf("resending the locked switch → %d, want 200", status)
	}
	if rows := settingRows(t, e, ai.PriceSyncKey); rows != 0 {
		t.Fatalf("the locked switch wrote %d rows, want none", rows)
	}
}

// Every role resolves the binding it serves through the boot value, over a
// binding an admin saved before the operator set one. Not parallel: the
// binding is the process's own.
func TestABootSetBindingIsTheOneAnInstallationServes(t *testing.T) {
	e := SetupSearch(t)
	store := ai.NewRoutingStore(compose.NewSettingsStore(e.Pool), compose.StoredConfig(e.Pool), config.Static(nil))
	if _, err := store.Replace(e.adminRoutingCtx(), parsedRouting(t, "fake-stored")); err != nil {
		t.Fatalf("storing the binding: %v", err)
	}
	apptest.BindEnv(t, []string{envBinding("fake-env")})

	cfg, err := compose.ResolveRouting(context.Background(), e.Pool, "", config.Static(nil), discard())
	if err != nil {
		t.Fatalf("ResolveRouting: %v", err)
	}
	if got := cfg.Tiers[ai.TierPremium].Model; got != "fake-env" {
		t.Errorf("premium = %q, want the fake-env the environment sets over the saved fake-stored", got)
	}
}
