// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// An admin's operating values, saved through the installation-settings
// surface, are what the worker schedules and paces by: the save goes through
// the handler and the reads are the worker's own.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func installationAdminCtx(e *integration.Env) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + ids.NewV7().String(), UserID: ids.NewV7(),
		Permissions: principal.Permissions{
			Objects:  map[string]principal.ObjectGrant{"installation_settings": {Read: true, Update: true}},
			RowScope: principal.RowScopeAll,
		},
	})
}

// patchOperations sends one PATCH the way the settings screen does and
// returns the status and the operating values the response carries.
func patchOperations(t *testing.T, e *integration.Env, h installationSettingsHandlers, body string) (int, crmcontracts.OperationSettings) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPatch, "/v1/installation/settings", strings.NewReader(body)).
		WithContext(installationAdminCtx(e))
	rec := httptest.NewRecorder()
	h.UpdateInstallationSettings(rec, req)
	var out crmcontracts.InstallationSettings
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("decoding the PATCH answer: %v", err)
		}
	}
	return rec.Code, out.Operations
}

func TestTheWorkerSchedulesAndPacesByWhatAnAdminSaved(t *testing.T) {
	e := integration.Setup(t)
	h := installationSettingsHandlers{store: identity.NewInstallationSettings(e.DB(), NewSettingsStore(e.Pool))}
	// The settings are installation-wide and this database is shared, so the
	// defaults go back through the same surface when the test ends.
	t.Cleanup(func() {
		if code, _ := patchOperations(t, e, h, `{"send_rate_limit":30,"close_date_sweep_interval_seconds":86400,"geocode_backfill_interval_seconds":3600}`); code != http.StatusOK {
			t.Errorf("restoring the defaults → %d", code)
		}
	})

	code, saved := patchOperations(t, e, h, `{"send_rate_limit":5,"close_date_sweep_interval_seconds":7200,"geocode_backfill_interval_seconds":0}`)
	if code != http.StatusOK {
		t.Fatalf("saving the operating values → %d", code)
	}
	if saved.SendRateLimit != 5 || saved.CloseDateSweepIntervalSeconds != 7200 || saved.GeocodeBackfillIntervalSeconds != 0 {
		t.Fatalf("the save answered %+v, want the values sent", saved)
	}

	pace, err := readSendPace(SendWorkerContext(context.Background(), e.WS), NewSettingsStore(e.Pool))
	if err != nil {
		t.Fatalf("reading the pace as the send worker: %v", err)
	}
	if pace.limit != 5 {
		t.Errorf("the send worker paces at %d messages, want the 5 an admin saved", pace.limit)
	}

	book, err := ReadSchedules(context.Background(), e.Pool)
	if err != nil {
		t.Fatalf("reading the schedules as the worker does: %v", err)
	}
	if got := book.intervals[identity.CloseDateSweepIntervalSeconds.Key()].get(); got != 2*time.Hour {
		t.Errorf("the close-date sweep is scheduled every %s, want the 2h an admin saved", got)
	}
	if got := book.intervals[identity.GeocodeBackfillIntervalSeconds.Key()].get(); got != 0 {
		t.Errorf("the address sweep is scheduled every %s, want it off", got)
	}
}

// Zero is the off an address sweep admits, and anything from one to just
// under five minutes is refused rather than run as a near-busy loop.
func TestAnOperatingValueOutsideItsRangeIsRefused(t *testing.T) {
	e := integration.Setup(t)
	h := installationSettingsHandlers{store: identity.NewInstallationSettings(e.DB(), NewSettingsStore(e.Pool))}
	for _, body := range []string{
		`{"geocode_backfill_interval_seconds":100}`,
		`{"retention_sweep_interval_seconds":0}`,
		`{"send_rate_limit":0}`,
	} {
		if code, _ := patchOperations(t, e, h, body); code != http.StatusUnprocessableEntity {
			t.Errorf("PATCH %s → %d, want 422", body, code)
		}
	}
}
