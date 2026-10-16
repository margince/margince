// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/config"
)

const lockedCode = "setting_locked_by_boot_source"

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
// appears nowhere in the answer. Reading every setting is not enough to see it.
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

	actAsSettingsReader(t, e)
	if status := e.Call(t, http.MethodGet, "/v1/ai/budget", nil, nil, nil); status != http.StatusOK {
		t.Fatalf("the settings reader reading the budget → %d, want 200", status)
	}
	if status := e.Call(t, http.MethodGet, "/v1/installation/configuration", nil, nil, nil); status != http.StatusForbidden {
		t.Fatalf("a seat that reads every setting but may not change the installation → %d, want 403", status)
	}
}

// actAsSettingsReader signs this session in as an administrator without the
// update grant on installation_settings. It passes every stored setting's read
// gate, so only the overview's own gate can refuse it.
func actAsSettingsReader(t *testing.T, e *apptest.AppEnv) {
	t.Helper()
	if _, err := e.Owner.Exec(context.Background(), `
		INSERT INTO role (key, name, permissions)
		SELECT 'settings_reader', 'Settings reader',
		       jsonb_set(permissions, '{objects,installation_settings,update}', 'false')
		FROM role WHERE key = 'admin'`); err != nil {
		t.Fatalf("seeding the settings reader role: %v", err)
	}
	assignSessionRole(t, e, "settings_reader")
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
