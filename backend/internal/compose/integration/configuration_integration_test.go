// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/config"
)

const lockedCode = "setting_locked_by_boot_source"

// lockedSurface is a settings response, a value the environment sets on one of
// its settings, and a write that would change that setting. A write that must
// first read a revision has none here; its own lock test sends it.
type lockedSurface struct {
	contractPath string
	path         string
	key          string
	env, value   string
	write        lockedWrite
}

const (
	gmailEnvClientID = "env-client.apps.googleusercontent.com"
	gmailEnvSecret   = "GOCSPX-from-the-environment"
)

func lockedSurfaces() []lockedSurface {
	routing := strings.TrimPrefix(envBinding("env-model"), "MARGINCE_AI_ROUTING_BINDING=")
	return []lockedSurface{
		{
			"/capture/settings", "/v1/capture/settings", "capture.auto_enrich_daily_cap", "MARGINCE_CAPTURE_AUTO_ENRICH_DAILY_CAP", "900",
			lockedWrite{method: http.MethodPatch, path: "/v1/capture/settings", body: map[string]any{"auto_enrich_daily_cap": 5}},
		},
		{
			"/ai/routing", "/v1/ai/routing", "ai.routing", "MARGINCE_AI_ROUTING_BINDING", routing,
			lockedWrite{method: http.MethodPut, path: "/v1/ai/provider-settings/openai_compatible", body: map[string]any{"base_url": "https://openrouter.ai/api"}},
		},
		{
			"/ai/provider-keys", "/v1/ai/provider-keys", "ai.provider_keys_anthropic", "ANTHROPIC_API_KEY", envProviderKey,
			lockedWrite{method: http.MethodPut, path: "/v1/ai/provider-keys/anthropic", body: map[string]any{"api_key": envProviderKey}},
		},
		{
			"/ai/budget", "/v1/ai/budget", "ai.budget", "MARGINCE_AI_BUDGET", `{"tokens_per_full_user":5000,"company_monthly_tokens":777}`,
			lockedWrite{},
		},
		{
			"/ai/price-sync", "/v1/ai/price-sync", "ai.price_sync", "MARGINCE_AI_PRICE_SYNC", `{"auto_sync":false}`,
			lockedWrite{method: http.MethodPut, path: "/v1/ai/price-sync", body: map[string]any{"auto_sync": true}},
		},
		{
			"/integrations/settings", "/v1/integrations/settings", "integrations.automatic_lookup", "MARGINCE_INTEGRATIONS_AUTOMATIC_LOOKUP", "false",
			lockedWrite{method: http.MethodPatch, path: "/v1/integrations/settings", body: map[string]any{"automatic_lookup": true}},
		},
		{
			"/leads/settings", "/v1/leads/settings", "contacts.first_response_target_minutes", "MARGINCE_CONTACTS_FIRST_RESPONSE_TARGET_MINUTES", "60",
			lockedWrite{method: http.MethodPatch, path: "/v1/leads/settings", body: map[string]any{"first_response_target_minutes": 90}},
		},
		{
			"/email-signature-template", "/v1/email-signature-template", "contacts.signature_template", "MARGINCE_CONTACTS_SIGNATURE_TEMPLATE", "<p>{name}</p>",
			lockedWrite{method: http.MethodPut, path: "/v1/email-signature-template", body: map[string]any{"template": "<p>{title}</p>"}},
		},
		{
			"/retention/settings", "/v1/retention/settings", "privacy.retain_only", "MARGINCE_PRIVACY_RETAIN_ONLY", "true",
			lockedWrite{method: http.MethodPatch, path: "/v1/retention/settings", body: map[string]any{"retain_only": false}},
		},
		{
			"/activities/follow-up-settings", "/v1/activities/follow-up-settings", "activities.follow_up_after_days", "MARGINCE_ACTIVITIES_FOLLOW_UP_AFTER_DAYS", "6",
			lockedWrite{method: http.MethodPatch, path: "/v1/activities/follow-up-settings", body: map[string]any{"follow_up_after_days": 3}},
		},
		{
			"/legal-reviews/settings", "/v1/legal-reviews/settings", "deals.legal_review_enabled", "MARGINCE_DEALS_LEGAL_REVIEW_ENABLED", "false",
			lockedWrite{method: http.MethodPut, path: "/v1/legal-reviews/settings", body: map[string]any{"enabled": true}},
		},
		{
			"/privacy/controller-particulars", "/v1/privacy/controller-particulars", "consent.controller_particulars", "MARGINCE_CONSENT_CONTROLLER_PARTICULARS", `{"LegalName":"Env GmbH"}`,
			lockedWrite{method: http.MethodPut, path: "/v1/privacy/controller-particulars", body: map[string]any{"legal_name": "Other GmbH"}},
		},
		{
			"/installation/settings", "/v1/installation/settings", "installation.send_rate_limit", "MARGINCE_INSTALLATION_SEND_RATE_LIMIT", "7",
			lockedWrite{method: http.MethodPatch, path: "/v1/installation/settings", body: map[string]any{"send_rate_limit": 8}},
		},
		{
			"/installation/oauth-apps/{provider}", "/v1/installation/oauth-apps/google", "capture.google_app", "MARGINCE_GMAIL_CLIENT_ID", gmailEnvClientID,
			lockedWrite{method: http.MethodDelete, path: "/v1/installation/oauth-apps/google"},
		},
	}
}

// surfacesWithLocks are the GET paths whose answer carries `locks`, read from
// the contract, so a surface that gains the field joins the test below.
func surfacesWithLocks(t *testing.T) []string {
	t.Helper()
	doc, err := openapi3.NewLoader().LoadFromFile("../../../api/crm.yaml")
	if err != nil {
		t.Fatalf("loading the contract: %v", err)
	}
	var paths []string
	for path, item := range doc.Paths.Map() {
		if item.Get == nil {
			continue
		}
		ok := item.Get.Responses.Status(http.StatusOK)
		if ok == nil || ok.Value == nil || ok.Value.Content.Get("application/json") == nil {
			continue
		}
		schema := ok.Value.Content.Get("application/json").Schema
		if schema != nil && schema.Value != nil && schema.Value.Properties["locks"] != nil {
			paths = append(paths, path)
		}
	}
	slices.Sort(paths)
	return paths
}

// Every settings response names the settings a boot source sets, with the
// variable that sets them. Every write to one is refused with the lock's code.
func TestEverySettingsResponseReportsItsLocks(t *testing.T) {
	surfaces := lockedSurfaces()
	var covered, environ []string
	for _, s := range surfaces {
		covered = append(covered, s.contractPath)
		environ = append(environ, s.env+"="+s.value)
	}
	slices.Sort(covered)
	if declared := surfacesWithLocks(t); !slices.Equal(declared, covered) {
		t.Fatalf("the contract gives locks to %v; this test covers %v", declared, covered)
	}
	apptest.BindEnv(t, append(environ, "MARGINCE_GMAIL_CLIENT_SECRET="+gmailEnvSecret))
	e, _ := apptest.SetupGoogleConnect(t, gmailEnvClientID, gmailEnvSecret)

	for _, s := range surfaces {
		var read struct {
			Locks []crmcontracts.SettingLock `json:"locks"`
		}
		if status := e.Call(t, http.MethodGet, s.path, nil, nil, &read); status != http.StatusOK {
			t.Errorf("GET %s → %d", s.path, status)
			continue
		}
		i := slices.IndexFunc(read.Locks, func(l crmcontracts.SettingLock) bool { return l.Key == s.key })
		if i < 0 || read.Locks[i].Origin.Source != crmcontracts.SettingOriginSourceEnv || read.Locks[i].Origin.Name != s.env {
			t.Errorf("GET %s locks %+v, want %s locked by env %s", s.path, read.Locks, s.key, s.env)
		}
		if s.write.method == "" {
			continue
		}
		var refused problemBody
		if status := e.Call(t, s.write.method, s.write.path, s.write.body, nil, &refused); status != http.StatusConflict || refused.Code != lockedCode {
			t.Errorf("%s %s → %d %q, want 409 %s", s.write.method, s.write.path, status, refused.Code, lockedCode)
		}
	}
}

// A settings response carries the value a boot source sets, and its lock names
// the source. Only a change is refused: the value in force, sent again beside
// another field, saves.
func TestLockedSettingsShowInTheAPIAndRefuseAChange(t *testing.T) {
	e := apptest.SetupAppWithEnv(t, []string{"MARGINCE_CAPTURE_AUTO_ENRICH_DAILY_CAP=900"})
	e.BootstrapWorkspace(t)

	const path = "/v1/capture/settings"
	var read crmcontracts.CaptureSettings
	if status := e.Call(t, http.MethodGet, path, nil, nil, &read); status != http.StatusOK || read.AutoEnrichDailyCap != 900 {
		t.Fatalf("GET %s → %d, cap %d, want the 900 the environment sets", path, status, read.AutoEnrichDailyCap)
	}
	if read.Locks == nil || len(*read.Locks) != 1 || (*read.Locks)[0].Key != "capture.auto_enrich_daily_cap" ||
		(*read.Locks)[0].Origin.Source != crmcontracts.SettingOriginSourceEnv ||
		(*read.Locks)[0].Origin.Name != "MARGINCE_CAPTURE_AUTO_ENRICH_DAILY_CAP" {
		t.Fatalf("locks %+v, want the cap locked by MARGINCE_CAPTURE_AUTO_ENRICH_DAILY_CAP", read.Locks)
	}
	var refused problemBody
	if status := e.Call(t, http.MethodPatch, path, map[string]any{"auto_enrich_daily_cap": 5}, nil, &refused); status != http.StatusConflict ||
		refused.Code != lockedCode || !strings.Contains(refused.Detail, "MARGINCE_CAPTURE_AUTO_ENRICH_DAILY_CAP") {
		t.Fatalf("a changed cap → %d %+v, want 409 %s naming the variable", status, refused, lockedCode)
	}
	if status := e.Call(t, http.MethodPatch, path, map[string]any{"auto_enrich_daily_cap": 900, "auto_enrich": false}, nil, &read); status != http.StatusOK || read.AutoEnrich {
		t.Fatalf("the locked cap resent beside a change → %d, auto_enrich %v, want 200 and false saved", status, read.AutoEnrich)
	}
}

// The overview lists every setting of every program once, with the programs
// that read it. A secret set at boot is named by its source, and its value
// appears nowhere in the answer.
func TestTheOverviewNeverCarriesASecretValue(t *testing.T) {
	const webhookKey = "c2VjcmV0c2VjcmV0c2VjcmV0c2VjcmV0c2VjcmV0MTI="
	e := apptest.SetupAppWithEnv(t, []string{
		"MARGINCE_WEBHOOK_KEY=" + webhookKey, "ANTHROPIC_API_KEY=" + envProviderKey, "MARGINCE_CAPTURE_AUTO_ENRICH_DAILY_CAP=900",
	})
	e.BootstrapWorkspace(t)

	var raw json.RawMessage
	if status := e.Call(t, http.MethodGet, "/v1/installation/configuration", nil, nil, &raw); status != http.StatusOK {
		t.Fatalf("GET /installation/configuration → %d", status)
	}
	for _, secret := range []string{webhookKey, envProviderKey} {
		if strings.Contains(string(raw), secret) {
			t.Fatalf("the overview carries the secret %q", secret)
		}
	}
	var entries []crmcontracts.ConfigurationEntry
	if err := json.Unmarshal(raw, &entries); err != nil {
		t.Fatalf("decoding the overview: %v", err)
	}
	byKey := overviewByKey(t, entries)
	for key, env := range map[string]string{"webhook_key": "MARGINCE_WEBHOOK_KEY", "ai.provider_keys_anthropic": "ANTHROPIC_API_KEY"} {
		got := byKey[key]
		if !got.Secret || got.Value != nil || got.Default != nil || got.Origin.Source != crmcontracts.SettingOriginSourceEnv || got.Origin.Name != env {
			t.Errorf("%s reads %+v, want a secret with no value, set by env %s", key, got, env)
		}
	}
	if daily := byKey["capture.auto_enrich_daily_cap"]; daily.Value == nil || *daily.Value != "900" || daily.Origin.Name != "MARGINCE_CAPTURE_AUTO_ENRICH_DAILY_CAP" {
		t.Errorf("the boot-set cap reads %+v, want 900 from its variable", daily)
	}

	demoteToRep(t, e)
	if status := e.Call(t, http.MethodGet, "/v1/installation/configuration", nil, nil, nil); status != http.StatusForbidden {
		t.Fatalf("a rep reading the overview → %d, want 403", status)
	}
}

// overviewByKey indexes the overview, and fails unless it is sorted and lists
// every program's settings once, each with the programs whose registry holds it.
func overviewByKey(t *testing.T, entries []crmcontracts.ConfigurationEntry) map[string]crmcontracts.ConfigurationEntry {
	t.Helper()
	if !slices.IsSortedFunc(entries, func(a, b crmcontracts.ConfigurationEntry) int { return strings.Compare(a.Key, b.Key) }) {
		t.Fatal("the overview is not sorted by key")
	}
	want := map[string][]crmcontracts.ConfigurationEntryRoles{}
	registries := compose.ProgramRegistries()
	for _, role := range []config.Role{config.RoleAPI, config.RoleWorker, config.RoleMigrate} {
		for _, s := range registries[role].All() {
			want[s.Key()] = append(want[s.Key()], crmcontracts.ConfigurationEntryRoles(role))
		}
	}
	byKey := make(map[string]crmcontracts.ConfigurationEntry, len(entries))
	for _, entry := range entries {
		if _, twice := byKey[entry.Key]; twice {
			t.Fatalf("the overview lists %s twice", entry.Key)
		}
		byKey[entry.Key] = entry
		if !slices.Equal(entry.Roles, want[entry.Key]) {
			t.Errorf("%s is read by %v, want %v", entry.Key, entry.Roles, want[entry.Key])
		}
	}
	if len(byKey) != len(want) {
		t.Fatalf("the overview lists %d settings, the programs register %d", len(byKey), len(want))
	}
	return byKey
}

// The task overrides answer as a map of tasks, with no room for a lock. Their
// lock is reported on the routing response, which the same screen reads.
func TestTheRoutingResponseReportsTheTaskOverridesLock(t *testing.T) {
	e := apptest.SetupAppWithEnv(t, []string{`MARGINCE_AI_TASK_OVERRIDES={"capture_classify":{"thinking":"high"}}`})
	e.BootstrapWorkspace(t)

	var read crmcontracts.AiRouting
	if status := e.Call(t, http.MethodGet, "/v1/ai/routing", nil, nil, &read); status != http.StatusOK || read.Locks == nil {
		t.Fatalf("GET /ai/routing → %d, locks %v", status, read.Locks)
	}
	want := []crmcontracts.SettingLock{{
		Key:    "ai.task_overrides",
		Origin: crmcontracts.SettingOrigin{Source: crmcontracts.SettingOriginSourceEnv, Name: "MARGINCE_AI_TASK_OVERRIDES"},
	}}
	if got := *read.Locks; len(got) != 1 || got[0].Key != want[0].Key || got[0].Origin.Source != want[0].Origin.Source || got[0].Origin.Name != want[0].Origin.Name {
		t.Fatalf("routing locks %+v, want %+v", got, want)
	}
}
