// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A locked field answers 409 whether or not the value sent is valid, and the
// reads with no save behind them answer the boot value too.

import (
	"crypto/rand"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/platform/keyvault"
)

func TestAnInvalidChangeOfALockedBindingIsRefusedAsLocked(t *testing.T) {
	e := apptest.SetupAppWithEnv(t, []string{envBinding("env-model")})
	e.BootstrapWorkspace(t)
	var read json.RawMessage
	if status := e.Call(t, http.MethodGet, "/v1/ai/routing", nil, nil, &read); status != http.StatusOK {
		t.Fatalf("GET /ai/routing → %d", status)
	}
	refusedByTheStore := strings.Replace(string(read), `"env-model"`, `""`, 1)
	refusedByTheHandler := strings.Replace(string(read), `"model":"env-model"`, `"model":"env-model","routing":{"nonsense":true}`, 1)
	for name, body := range map[string]string{"an empty model": refusedByTheStore, "an unknown routing key": refusedByTheHandler} {
		if status := e.Call(t, http.MethodPut, "/v1/ai/routing", json.RawMessage(body), nil, nil); status != http.StatusConflict {
			t.Errorf("%s in the locked binding → %d, want 409", name, status)
		}
	}
	if status := e.Call(t, http.MethodPut, "/v1/ai/provider-settings/openai_compatible",
		map[string]string{"base_url": "not a url"}, nil, nil); status != http.StatusConflict {
		t.Errorf("an invalid provider host under the locked binding → %d, want 409", status)
	}
}

// A resend is held to the revision the GET's ETag names. So the editor's
// If-Match save of the locked binding it read is a no-op.
func TestAnIfMatchResendOfALockedBindingIsANoOp(t *testing.T) {
	env := envBinding("env-model")
	e := apptest.SetupAppWithEnv(t, []string{env})
	e.BootstrapWorkspace(t)
	var boot ai.RoutingConfig
	if err := json.Unmarshal([]byte(strings.TrimPrefix(env, "MARGINCE_AI_ROUTING_BINDING=")), &boot); err != nil {
		t.Fatal(err)
	}
	var read json.RawMessage
	if status := e.Call(t, http.MethodGet, "/v1/ai/routing", nil, nil, &read); status != http.StatusOK {
		t.Fatalf("GET /ai/routing → %d", status)
	}
	ifMatch := map[string]string{"If-Match": `"` + boot.Revision() + `"`}
	if status := e.Call(t, http.MethodPut, "/v1/ai/routing", read, ifMatch, nil); status != http.StatusOK {
		t.Fatalf("an If-Match resend of the locked binding → %d, want 200", status)
	}
}

func TestAnInvalidChangeOfLockedOverridesIsRefusedAsLocked(t *testing.T) {
	e := apptest.SetupAppWithEnv(t, []string{`MARGINCE_AI_TASK_OVERRIDES={"capture_classify":{"thinking":"high"}}`})
	e.BootstrapWorkspace(t)
	for name, body := range map[string]string{
		"a timeout written empty":     `{"capture_classify":{"thinking":"high","attempt_timeout_ms":0}}`,
		"a timeout below its bounds":  `{"capture_classify":{"thinking":"high","attempt_timeout_ms":1}}`,
		"a task this build never ran": `{"capture_classify":{"thinking":"high"},"no_such_task":{"thinking":"low"}}`,
	} {
		if status := e.Call(t, http.MethodPut, "/v1/ai/task-overrides", json.RawMessage(body), nil, nil); status != http.StatusConflict {
			t.Errorf("%s in the locked overrides → %d, want 409", name, status)
		}
	}
}

func TestAnInvalidChangeOfALockedAllowanceIsRefusedAsLocked(t *testing.T) {
	e := apptest.SetupAppWithEnv(t, []string{`MARGINCE_AI_BUDGET={"tokens_per_full_user":5000,"company_monthly_tokens":777}`})
	e.BootstrapWorkspace(t)
	var read struct {
		Revision string `json:"revision"`
	}
	if status := e.Call(t, http.MethodGet, "/v1/ai/budget", nil, nil, &read); status != http.StatusOK {
		t.Fatalf("GET /ai/budget → %d", status)
	}
	change := map[string]any{
		"config":            map[string]any{"tokens_per_full_user": 0, "company_monthly_tokens": nil},
		"expected_revision": read.Revision,
	}
	if status := e.Call(t, http.MethodPut, "/v1/ai/budget", change, nil, nil); status != http.StatusConflict {
		t.Fatalf("an allowance of no tokens under the locked one → %d, want 409", status)
	}
}

// A name, a country or a send limit the environment sets refuses an invalid
// change as locked, not as a value the screen could correct. The country is
// the registered test pack zk.
func TestAnInvalidChangeOfALockedInstallationSettingIsRefusedAsLocked(t *testing.T) {
	e := apptest.SetupAppWithEnv(t, []string{
		"MARGINCE_INSTALLATION_NAME=Env Name Co", "MARGINCE_INSTALLATION_COUNTRY=zk", "MARGINCE_INSTALLATION_SEND_RATE_LIMIT=7",
	})
	e.BootstrapWorkspace(t)
	for name, patch := range map[string]map[string]any{
		"a rename":                  {"name": "Other Co"},
		"a country with no pack":    {"country": "zzz"},
		"a limit below its minimum": {"send_rate_limit": 0},
	} {
		if status := e.Call(t, http.MethodPatch, "/v1/installation/settings", patch, nil, nil); status != http.StatusConflict {
			t.Errorf("%s of a locked setting → %d, want 409", name, status)
		}
	}
}

// The status reads the binding, the overrides and the allowance the
// environment sets, though no save ever wrote them.
func TestTheStatusReadsTheBootValues(t *testing.T) {
	env := envBinding("env-model")
	e := apptest.SetupAppWithEnv(t, []string{
		env,
		`MARGINCE_AI_TASK_OVERRIDES={"capture_classify":{"thinking":"high"}}`,
		`MARGINCE_AI_BUDGET={"tokens_per_full_user":5000,"company_monthly_tokens":777}`,
	})
	e.BootstrapWorkspace(t)
	var boot ai.RoutingConfig
	if err := json.Unmarshal([]byte(strings.TrimPrefix(env, "MARGINCE_AI_ROUTING_BINDING=")), &boot); err != nil {
		t.Fatal(err)
	}
	var status struct {
		RoutingVersion string `json:"routing_version"`
		Budget         struct {
			MonthlyTokens int64 `json:"monthly_tokens"`
		} `json:"budget"`
		Features []struct {
			Task      string `json:"task"`
			Overrides *struct {
				Thinking string `json:"thinking"`
			} `json:"overrides"`
		} `json:"features"`
	}
	if code := e.Call(t, http.MethodGet, "/v1/ai/status", nil, nil, &status); code != http.StatusOK {
		t.Fatalf("GET /ai/status → %d", code)
	}
	if status.RoutingVersion != boot.Revision() || status.Budget.MonthlyTokens != 777 {
		t.Errorf("status reads routing %q and %d tokens, want the boot %q and 777", status.RoutingVersion, status.Budget.MonthlyTokens, boot.Revision())
	}
	for _, row := range status.Features {
		if row.Task == "capture_classify" && (row.Overrides == nil || row.Overrides.Thinking != "high") {
			t.Errorf("capture_classify reads overrides %+v, want the boot high", row.Overrides)
		}
	}
}

// Whether a Vertex key may be checked follows the boot binding's profile: a
// sovereign one forbids reaching Google, though no saved binding says so.
func TestTheKeyCheckFollowsTheBootBindingsProfile(t *testing.T) {
	lane := `{"provider":"ollama","model":"local"}`
	root := make([]byte, 32)
	if _, err := rand.Read(root); err != nil {
		t.Fatal(err)
	}
	vault, err := keyvault.New(keyvault.Config{RootKey: root, Pool: apptest.EarlyPool(t)})
	if err != nil {
		t.Fatal(err)
	}
	// The vault's own wiring builds the key store this test is about.
	e := apptest.SetupAppWithEnv(t, []string{`MARGINCE_AI_ROUTING_BINDING={"profile":"sovereign","tiers":{"local_small":` + lane +
		`,"cheap_cloud":` + lane + `,"premium":` + lane + `,"frontier":` + lane + `},` +
		`"embeddings":{"provider":"ollama","model":"local-embed","dimensions":8}}`}, compose.WithKeyvault(vault))
	e.BootstrapWorkspace(t)
	var refusal json.RawMessage
	status := e.Call(t, http.MethodPut, "/v1/ai/provider-keys/gemini_vertex",
		map[string]string{"service_account_json": `{"type":"service_account"}`}, nil, &refusal)
	if status != http.StatusUnprocessableEntity || !strings.Contains(string(refusal), "sovereign") {
		t.Fatalf("a Vertex key under a sovereign boot binding → %d %s, want 422 naming the profile", status, refusal)
	}
}
