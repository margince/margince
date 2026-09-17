// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// GET /admin/extension-ingest-health over the real wire.
//
// The breadcrumb is written by the ingress port, which is proved where it is
// written (compose/extingress_integration_test.go). What is proved HERE is the
// half the operator actually holds: that the rows reach a reader, folded per
// unit, over a window that ends stale counts — and that the endpoint is shut to
// everyone it is meant to be shut to.
//
// Rows are seeded rather than ingested on purpose: an ingest can only produce
// TODAY, and the window is the thing this file is about.

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

type extensionIngestHealthDTO struct {
	GeneratedAt string `json:"generated_at"`
	WindowDays  int    `json:"window_days"`
	Units       []struct {
		Unit          string  `json:"unit"`
		Refused       int     `json:"refused"`
		LastRefusedAt *string `json:"last_refused_at"`
		Refusals      []struct {
			Refusal       string  `json:"refusal"`
			Refused       int     `json:"refused"`
			LastRefusedAt *string `json:"last_refused_at"`
		} `json:"refusals"`
	} `json:"units"`
}

// seedRefusal writes one daily count, `daysAgo` days back.
func seedRefusal(t *testing.T, e *apptest.AppEnv, unit, refusal string, daysAgo, refused int) {
	t.Helper()
	at := time.Now().UTC().AddDate(0, 0, -daysAgo)
	if _, err := e.Owner.Exec(context.Background(),
		`INSERT INTO extension_ingest_refusal (unit, refusal, day, refused, first_at, last_at)
		 VALUES ($1, $2, $3::date, $4, $3, $3)`,
		unit, refusal, at, refused); err != nil {
		t.Fatalf("seeding a %s/%s refusal %d day(s) back: %v", unit, refusal, daysAgo, err)
	}
}

func TestExtensionIngestHealthFoldsPerUnitAndForgetsWhatIsOutsideTheWindow(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)

	// One unit failing two checks, on two days, so the fold has something to
	// add up and an order to get right.
	seedRefusal(t, e, "openchannel", "participants", 0, 40)
	seedRefusal(t, e, "openchannel", "participants", 2, 5)
	seedRefusal(t, e, "openchannel", "key", 1, 3)
	// A second unit, so a fold that only ever extended the last entry would
	// report one unit's refusals under the other's name.
	seedRefusal(t, e, "relaydemo", "activity", 1, 9)
	// Older than the window. A count that never ages out keeps naming a
	// mapping somebody fixed a month ago, which is how an operational page
	// stops being read.
	seedRefusal(t, e, "openchannel", "size", 30, 900)

	var report extensionIngestHealthDTO
	if status := e.Call(t, "GET", "/v1/admin/extension-ingest-health", nil, nil, &report); status != http.StatusOK {
		t.Fatalf("GET /admin/extension-ingest-health = %d, want 200", status)
	}

	if len(report.Units) != 2 {
		t.Fatalf("got %d units, want the two with anything refused in the window: %+v", len(report.Units), report.Units)
	}
	openchannel := report.Units[0]
	if openchannel.Unit != "openchannel" {
		t.Fatalf("first unit = %q, want openchannel", openchannel.Unit)
	}
	if openchannel.Refused != 48 {
		t.Errorf("openchannel refused = %d, want 48 (45 participants + 3 key) — a total that included "+
			"the 30-day-old row would read 948", openchannel.Refused)
	}
	if len(openchannel.Refusals) != 2 {
		t.Fatalf("openchannel classes = %+v, want participants and key only — `size` is outside the window", openchannel.Refusals)
	}
	if openchannel.Refusals[0].Refusal != "participants" || openchannel.Refusals[0].Refused != 45 {
		t.Errorf("classes are not largest-first with their own totals: %+v", openchannel.Refusals)
	}
	if openchannel.LastRefusedAt == nil {
		t.Error("openchannel reports no last_refused_at, so a reader cannot tell a live problem from a settled one")
	}
	if report.WindowDays <= 0 {
		t.Errorf("window_days = %d — a reader cannot interpret the counts without it", report.WindowDays)
	}
}

func TestAnInstallationWithNothingRefusedReportsNoUnitsRatherThanNoAnswer(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)

	var report extensionIngestHealthDTO
	if status := e.Call(t, "GET", "/v1/admin/extension-ingest-health", nil, nil, &report); status != http.StatusOK {
		t.Fatalf("GET /admin/extension-ingest-health = %d, want 200 — a healthy installation is an "+
			"answer, not a 404", status)
	}
	if len(report.Units) != 0 {
		t.Errorf("a clean installation reported %+v", report.Units)
	}
}

func TestExtensionIngestHealthIsShutToAnAgent(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	seedRefusal(t, e, "openchannel", "key", 0, 1)

	// An admin-minted read-scoped passport satisfies every object grant, so
	// the RBAC check alone would admit it. Which units are breaking is
	// operational knowledge about the installation rather than a record the
	// agent's human lent it, and the endpoint is declared human-only — so the
	// refusal has to come from the layer that does not depend on the grants.
	var minted struct {
		Token string `json:"token"`
	}
	if status := e.Call(t, "POST", "/v1/passports", AnyMap{
		"label": "extension ingest health probe", "scopes": []string{"read"},
	}, nil, &minted); status != http.StatusCreated {
		t.Fatalf("issue passport → %d", status)
	}

	status := e.Call(t, "GET", "/v1/admin/extension-ingest-health", nil,
		map[string]string{"Authorization": "Bearer " + minted.Token}, nil)
	if status != http.StatusForbidden {
		t.Fatalf("GET /admin/extension-ingest-health as an agent = %d, want 403", status)
	}
}
