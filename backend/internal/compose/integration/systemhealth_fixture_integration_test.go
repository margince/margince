// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"net/http"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestSystemHealthFixtureOverviewAgreesWithDiagnostics(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	var me crmcontracts.MeResponse
	if status := e.Call(t, "GET", "/v1/me", nil, nil, &me); status != http.StatusOK {
		t.Fatalf("me status %d", status)
	}
	workspace := apptest.InstallationWorkspaceUUID(t.Context(), t, e.Pool)
	ctx := principal.SystemActing(principal.WithWorkspaceID(t.Context(), workspace), "system:health_fixture")
	if err := compose.SeedSystemHealthFixture(ctx, e.Pool, ids.UUID(me.User.Id), string(me.User.Email), 3, clocktest.Now(t)); err != nil {
		t.Fatal(err)
	}
	var overview crmcontracts.SystemHealth
	var capture crmcontracts.CaptureHealth
	var intake crmcontracts.ExtensionIngestHealth
	var recovery crmcontracts.RecoveryHealth
	if status := e.Call(t, "GET", "/v1/admin/system-health", nil, nil, &overview); status != http.StatusOK {
		t.Fatalf("overview status %d", status)
	}
	if status := e.Call(t, "GET", "/v1/admin/capture-health", nil, nil, &capture); status != http.StatusOK {
		t.Fatalf("capture status %d", status)
	}
	if status := e.Call(t, "GET", "/v1/admin/extension-ingest-health", nil, nil, &intake); status != http.StatusOK {
		t.Fatalf("intake status %d", status)
	}
	if status := e.Call(t, "GET", "/v1/admin/recovery-health", nil, nil, &recovery); status != http.StatusOK {
		t.Fatalf("recovery status %d", status)
	}
	backlog := capture.Classifier.Pending + capture.Classifier.Unsure + capture.Classifier.Exhausted
	if backlog == 0 || len(intake.Units) != 1 || intake.Units[0].Unit != "health_fixture" || intake.Units[0].Refused != 3 {
		t.Fatalf("fixture evidence missing: capture=%+v intake=%+v", capture, intake)
	}
	if recovery.LastDrill == nil || recovery.LastDrill.Outcome != "passed" {
		t.Fatalf("passing recovery evidence missing: %+v", recovery)
	}
	assertFixtureHealthSummary(t, overview, backlog)
}

func assertFixtureHealthSummary(t *testing.T, overview crmcontracts.SystemHealth, backlog int) {
	t.Helper()
	classified, refused := 0, 0
	for _, finding := range overview.Findings {
		switch finding.Code {
		case "capture_processing", "capture_uncertain", "capture_exhausted":
			if finding.Count == nil {
				t.Fatalf("classifier finding has no count: %+v", finding)
			}
			classified += *finding.Count
		case "intake_refused":
			if finding.Count == nil || !finding.RequiresAction {
				t.Fatalf("refused intake must need review: %+v", finding)
			}
			refused += *finding.Count
		}
		if finding.Source == "recovery" {
			t.Fatalf("passed drill produced an alert: %+v", finding)
		}
	}
	if classified != backlog || refused != 3 {
		t.Fatalf("overview differs from diagnostics: classified=%d want=%d refused=%d", classified, backlog, refused)
	}
	verified := false
	for _, source := range overview.Sources {
		if source.Id == "recovery" {
			verified = source.Code == "recovery_verified" && source.Condition == "clear" && source.Availability == "available"
		}
		if source.Id == "capture" && source.Availability != "unknown" {
			t.Fatalf("unrun capture sweeps must remain unobserved: %+v", source)
		}
	}
	if !verified {
		t.Fatalf("passed recovery drill absent from overview: %+v", overview.Sources)
	}
}

func TestSystemHealthFixtureRejectsAnEmptyBenchmark(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	workspace := apptest.InstallationWorkspaceUUID(t.Context(), t, e.Pool)
	ctx := principal.SystemActing(principal.WithWorkspaceID(t.Context(), workspace), "system:health_fixture")
	err := compose.SeedSystemHealthFixture(ctx, e.Pool, ids.UUID{}, "ada@example.com", 0, clocktest.Now(t))
	if err == nil || !strings.Contains(err.Error(), "no classifier backlog") {
		t.Fatalf("empty benchmark was accepted: %v", err)
	}
	var recovery crmcontracts.RecoveryHealth
	if status := e.Call(t, "GET", "/v1/admin/recovery-health", nil, nil, &recovery); status != http.StatusOK {
		t.Fatalf("recovery status %d", status)
	}
	if recovery.LastDrill != nil {
		t.Fatalf("rejected fixture recorded a recovery drill: %+v", recovery)
	}
}
