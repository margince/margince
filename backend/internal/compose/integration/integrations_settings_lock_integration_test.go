// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/modules/integrations"
)

// The automatic-lookup posture the api's environment sets reads as that value
// and refuses a change; resending it is accepted.
func TestAutomaticLookupSetByEnvIsLockedAgainstAChange(t *testing.T) {
	e := apptest.SetupAppWithEnv(t, []string{"MARGINCE_INTEGRATIONS_AUTOMATIC_LOOKUP=false"})
	e.BootstrapWorkspace(t)

	var read struct {
		AutomaticLookup bool `json:"automatic_lookup"`
	}
	if status := e.Call(t, http.MethodGet, "/v1/integrations/settings", nil, nil, &read); status != http.StatusOK || read.AutomaticLookup {
		t.Fatalf("GET /integrations/settings → %d, automatic_lookup %v, want the false the environment sets", status, read.AutomaticLookup)
	}
	if status := e.Call(t, http.MethodPatch, "/v1/integrations/settings",
		map[string]any{"automatic_lookup": true}, nil, nil); status != http.StatusConflict {
		t.Fatalf("a change of the locked automatic_lookup → %d, want 409", status)
	}
	if status := e.Call(t, http.MethodPatch, "/v1/integrations/settings",
		map[string]any{"automatic_lookup": false}, nil, nil); status != http.StatusOK {
		t.Fatalf("resending the locked automatic_lookup → %d, want 200", status)
	}
	if rows := settingRows(t, e, integrations.AutomaticLookup.Key()); rows != 0 {
		t.Fatalf("the locked automatic_lookup wrote %d rows, want none", rows)
	}
}

// Admission reads the posture inside the write that queues a run, and a boot
// value of off wins there over a saved row of on. Not parallel: the binding is
// the process's own.
func TestABootSetLookupOffWinsOverTheSavedRowAndBuysNothing(t *testing.T) {
	apptest.BindEnv(t, []string{"MARGINCE_INTEGRATIONS_AUTOMATIC_LOOKUP=false"})
	c := setupProviderConsumer(t, true)

	if err := c.consumer.HandleEvent(context.Background(), humanCreated(c.contactID)); err != nil {
		t.Fatalf("a boot-set off surfaced as a failure: %v", err)
	}
	if runs := c.runsForContact(t); runs != 0 {
		t.Errorf("%d runs exist with automatic lookup off at boot, over a saved row of on", runs)
	}
}
