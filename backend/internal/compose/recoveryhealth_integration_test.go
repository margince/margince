// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/continuity"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func readRecoveryHealth(ctx context.Context, t *testing.T, e *integration.Env) (int, crmcontracts.RecoveryHealth) {
	t.Helper()
	h := recoveryHealthHandlers{
		ledger: continuity.NewStore(InstallationDB(e.Pool)),
		now:    func() time.Time { return time.Now().UTC() },
	}
	req := httptest.NewRequest(http.MethodGet, "/v1/admin/recovery-health", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	h.GetRecoveryHealth(rec, req)
	var out crmcontracts.RecoveryHealth
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("decoding the report: %v", err)
		}
	}
	return rec.Code, out
}

// An installation that never rehearsed says so with a null drill, beside the
// targets it has not yet been measured against.
func TestRecoveryHealthReportsNoDrillAsNull(t *testing.T) {
	e := integration.Setup(t)
	code, report := readRecoveryHealth(e.Admin(), t, e)
	if code != http.StatusOK {
		t.Fatalf("status %d, want 200", code)
	}
	if report.LastDrill != nil {
		t.Fatalf("an installation with no drill reports one: %+v", report.LastDrill)
	}
	if report.RecoveryTargetSeconds != 4*3600 || report.DataLossTargetSeconds != 3600 {
		t.Errorf("targets = %ds recovery / %ds data loss, want the published 4h / 1h",
			report.RecoveryTargetSeconds, report.DataLossTargetSeconds)
	}
}

// The page shows what the ledger measured: both windows come from the drill's
// own timestamps.
func TestRecoveryHealthReportsTheLatestDrillsMeasuredWindows(t *testing.T) {
	e := integration.Setup(t)
	ledger := continuity.NewStore(InstallationDB(e.Pool))
	id, err := ledger.Begin(e.Admin(), time.Now().Add(-40*time.Minute), "Dana Ops", "")
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if err := ledger.Finish(e.Admin(), id, continuity.OutcomeFailed, "restored copy would not start"); err != nil {
		t.Fatalf("Finish: %v", err)
	}

	code, report := readRecoveryHealth(e.Admin(), t, e)
	if code != http.StatusOK {
		t.Fatalf("status %d, want 200", code)
	}
	d := report.LastDrill
	if d == nil {
		t.Fatal("the recorded drill is missing from the report")
	}
	if d.Outcome != crmcontracts.RestoreDrillOutcomeFailed {
		t.Errorf("outcome = %q, want the failure that was recorded", d.Outcome)
	}
	if d.RecoverySeconds == nil {
		t.Error("a finished drill reports no recovery window")
	}
	if d.DataLossSeconds < 39*60 || d.DataLossSeconds > 41*60 {
		t.Errorf("data loss = %ds, want about forty minutes", d.DataLossSeconds)
	}
	if d.Operator != "Dana Ops" {
		t.Errorf("operator = %q, want the name the drill was recorded under", d.Operator)
	}
	if d.Notes == nil || *d.Notes != "restored copy would not start" {
		t.Errorf("notes = %v, want the closing note", d.Notes)
	}
}

// A seat without the operational grant is refused, as on every System health
// card, though every role may read installation settings.
func TestRecoveryHealthRefusesASeatWithoutTheOperationalGrant(t *testing.T) {
	e := integration.Setup(t)
	code, _ := readRecoveryHealth(e.As(e.Rep1, nil, integration.RepPerms), t, e)
	if code != http.StatusForbidden {
		t.Fatalf("a rep got %d, want 403", code)
	}
}

// The endpoint asks `job_health:read` and nothing else: an ops seat holding
// that grant without installation settings reads the report.
func TestRecoveryHealthAnswersASeatHoldingOnlyTheOperationalGrant(t *testing.T) {
	e := integration.Setup(t)
	ops := e.As(e.Rep1, nil, principal.Permissions{
		RoleKeys: []string{"ops"},
		Objects:  map[string]principal.ObjectGrant{"job_health": {Read: true}},
	})
	if code, _ := readRecoveryHealth(ops, t, e); code != http.StatusOK {
		t.Fatalf("an ops seat with job_health:read got %d, want 200", code)
	}
}
