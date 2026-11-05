// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

var callMessengerKeys = []string{"call_whatsapp", "call_telegram", "call_zalo"}

func readCallMessengers(t *testing.T, e *apptest.AppEnv) map[string]bool {
	t.Helper()
	var settings map[string]json.RawMessage
	if status := e.Call(t, http.MethodGet, "/v1/installation/settings", nil, nil, &settings); status != http.StatusOK {
		t.Fatalf("GET /v1/installation/settings = %d, want 200", status)
	}
	got := map[string]bool{}
	for _, key := range callMessengerKeys {
		var on bool
		if err := json.Unmarshal(settings[key], &on); err != nil {
			t.Fatalf("GET /v1/installation/settings carries %s = %s, want a boolean: %v", key, settings[key], err)
		}
		got[key] = on
	}
	return got
}

func callMessengerAudits(t *testing.T, e *apptest.AppEnv, key string) int {
	t.Helper()
	var n int
	if err := e.Owner.QueryRow(t.Context(),
		`SELECT count(*) FROM audit_log WHERE entity_type = 'installation_settings' AND after ? $1`,
		"installation."+key).Scan(&n); err != nil {
		t.Fatalf("counting audit rows for %s: %v", key, err)
	}
	return n
}

func patchCallMessenger(t *testing.T, e *apptest.AppEnv, key string, on bool) int {
	t.Helper()
	return e.Call(t, http.MethodPatch, "/v1/installation/settings", AnyMap{key: on}, nil, nil)
}

func TestCallMessengersAreOnByDefaultAndEachSwitchMovesOnlyItsOwnKey(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)

	for key, on := range readCallMessengers(t, e) {
		if !on {
			t.Errorf("an untouched installation reads %s = false, want the default true", key)
		}
	}

	for _, key := range callMessengerKeys {
		before := callMessengerAudits(t, e, key)
		if status := patchCallMessenger(t, e, key, false); status != http.StatusOK {
			t.Fatalf("admin PATCH {%s: false} = %d, want 200", key, status)
		}
		for other, on := range readCallMessengers(t, e) {
			if want := other != key; on != want {
				t.Errorf("after PATCH {%s: false}, %s = %v, want %v", key, other, on, want)
			}
		}
		if after := callMessengerAudits(t, e, key); after != before+1 {
			t.Errorf("PATCH {%s: false} wrote %d audit rows for it, want 1", key, after-before)
		}
		if status := patchCallMessenger(t, e, key, true); status != http.StatusOK {
			t.Fatalf("admin PATCH {%s: true} = %d, want 200", key, status)
		}
	}

	demoteToRep(t, e)
	for _, key := range callMessengerKeys {
		before := callMessengerAudits(t, e, key)
		if status := patchCallMessenger(t, e, key, false); status != http.StatusForbidden {
			t.Errorf("rep PATCH {%s: false} = %d, want 403", key, status)
		}
		if after := callMessengerAudits(t, e, key); after != before {
			t.Errorf("a refused rep PATCH {%s: false} wrote %d audit rows", key, after-before)
		}
	}
}
