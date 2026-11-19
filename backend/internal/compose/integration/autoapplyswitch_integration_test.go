// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The installation switch over every rep's own automatic setting. The sweep is
// the real one, so the refusal cannot pass by the switch being read somewhere
// the applier does not look.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/config/configtest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// setInstallationAutoApply flips the switch as an admin does, through its real
// writer.
func setInstallationAutoApply(t *testing.T, e *Env, on bool) {
	t.Helper()
	t.Cleanup(func() { e.WsExec(t, `DELETE FROM setting WHERE key = $1`, approvals.AutoApplyEnabled.Key()) })
	cfg := configtest.StoredOver(t, e.Pool, approvals.ConfigSettings())
	admin := e.As(e.AdminUser, nil, principal.Permissions{
		RoleKeys: []string{"admin"}, RowScope: principal.RowScopeAll,
		Objects: map[string]principal.ObjectGrant{"installation_settings": {Read: true, Update: true}},
	})
	if err := approvals.SetAutoApplyEnabled(admin, cfg, on); err != nil {
		t.Fatalf("switching the installation to %v: %v", on, err)
	}
}

func sweepApplied(ctx context.Context, t *testing.T, e *Env) int {
	t.Helper()
	applied, err := compose.SweepAutoApply(ctx, e.Pool)
	if err != nil {
		t.Fatalf("sweeping: %v", err)
	}
	return applied
}

// screenBound is what the rep's GET /autonomy says about the installation switch.
func screenBound(ctx context.Context, t *testing.T, svc *approvals.Service) bool {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/autonomy", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	approvals.NewHandlers(svc).GetAutonomy(rec, req)
	var body struct {
		On bool `json:"auto_apply_enabled"`
	}
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &body) != nil {
		t.Fatalf("GET /autonomy answered %d: %s", rec.Code, rec.Body.String())
	}
	return body.On
}

func TestNothingAppliesWhileTheInstallationSwitchIsOff(t *testing.T) {
	e := Setup(t)
	pipeline, open, _ := DealFixture(t, e)
	svc := approvals.NewService(e.DB())
	deal := e.SeedDeal(t, "Fleet retrofit", pipeline, open, &e.Rep1)
	grantDealRepRole(t, e, e.Rep1)
	turnAutoApplyOn(t, svc, e, e.Rep1)
	approvalID := stageCloseDateCorrection(t, svc, e, deal)

	setInstallationAutoApply(t, e, false)
	if got := sweepApplied(sweepCtx(e), t, e); got != 0 {
		t.Fatalf("applied %d proposals with the installation switch off, want none", got)
	}
	if status, _ := statusOf(t, approvalID); status != "pending" {
		t.Fatalf("status = %q, want the proposal still waiting", status)
	}

	repCtx := e.As(e.Rep1, []ids.UUID{e.Team1}, RepPerms)
	if screenBound(repCtx, t, svc) {
		t.Fatal("the rep's screen reports the installation switch on, want off")
	}
	rows, err := svc.AutoApplySettings(repCtx)
	if err != nil {
		t.Fatalf("reading the rep's settings: %v", err)
	}
	if row := kindRow(t, rows, deals.CloseDateCorrectionKind); row.Mode != approvals.ModeAuto {
		t.Errorf("the rep's own mode reads %q while the bound is off, want auto kept", row.Mode)
	}

	setInstallationAutoApply(t, e, true)
	if got := sweepApplied(sweepCtx(e), t, e); got != 1 {
		t.Fatalf("applied %d proposals after the switch came back on, want the rep's automatic kind to apply", got)
	}
}

func TestTurningTheInstallationSwitchOnLeavesAManualRepManual(t *testing.T) {
	e := Setup(t)
	pipeline, open, _ := DealFixture(t, e)
	svc := approvals.NewService(e.DB())
	deal := e.SeedDeal(t, "Fleet retrofit", pipeline, open, &e.Rep1)
	grantDealRepRole(t, e, e.Rep1)
	repCtx := e.As(e.Rep1, []ids.UUID{e.Team1}, RepPerms)
	if _, err := svc.SetAutoApply(repCtx, deals.CloseDateCorrectionKind, false); err != nil {
		t.Fatalf("switching the kind off: %v", err)
	}
	approvalID := stageCloseDateCorrection(t, svc, e, deal)

	setInstallationAutoApply(t, e, false)
	setInstallationAutoApply(t, e, true)
	if got := sweepApplied(sweepCtx(e), t, e); got != 0 {
		t.Fatalf("applied %d proposals for a rep on manual after the switch went on, want none", got)
	}
	if status, _ := statusOf(t, approvalID); status != "pending" {
		t.Fatalf("status = %q, want the proposal still waiting", status)
	}
}
