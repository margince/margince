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
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/config/configtest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

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

	SetAutoApplyEnabled(t, e, false)
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

	SetAutoApplyEnabled(t, e, true)
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

	SetAutoApplyEnabled(t, e, false)
	SetAutoApplyEnabled(t, e, true)
	if got := sweepApplied(sweepCtx(e), t, e); got != 0 {
		t.Fatalf("applied %d proposals for a rep on manual after the switch went on, want none", got)
	}
	if status, _ := statusOf(t, approvalID); status != "pending" {
		t.Fatalf("status = %q, want the proposal still waiting", status)
	}
}

// A switch-off must wait for a transaction that already read "on", so nothing
// applies after the admin's change has returned.
func TestTheSwitchOffWaitsForAnApplyAlreadyInFlight(t *testing.T) {
	e := Setup(t)
	ctx := e.Admin()
	done := make(chan error, 1)
	waiting := func() bool {
		var n int
		err := OwnerConn(t).QueryRow(context.Background(),
			`SELECT count(*) FROM pg_locks WHERE locktype = 'advisory' AND NOT granted`).Scan(&n)
		if err != nil {
			t.Fatalf("reading lock waits: %v", err)
		}
		return n > 0
	}
	err := database.WithTx(ctx, e.Pool, func(tx pgx.Tx) error {
		mode, err := approvals.AutoApplyModeTx(ctx, tx, e.Rep1, deals.CloseDateCorrectionKind)
		if err != nil || mode != approvals.ModeAuto {
			t.Fatalf("in-flight read = %q, %v; want auto", mode, err)
		}
		go func() {
			SetAutoApplyEnabled(t, e, false)
			done <- nil
		}()
		patience, stop := context.WithTimeout(context.Background(), 20*time.Second)
		defer stop()
		for !waiting() {
			if patience.Err() != nil {
				t.Fatal("the switch-off never queued behind the in-flight apply")
			}
			runtime.Gosched()
		}
		select {
		case <-done:
			t.Fatal("the switch-off returned while an apply that read on was still open")
		default:
		}
		return nil
	})
	if err != nil {
		t.Fatalf("in-flight transaction: %v", err)
	}
	<-done
	err = database.WithTx(ctx, e.Pool, func(tx pgx.Tx) error {
		mode, err := approvals.AutoApplyModeTx(ctx, tx, e.Rep1, deals.CloseDateCorrectionKind)
		if mode != approvals.ModeManual {
			t.Errorf("after the switch-off returned, mode = %q, want manual", mode)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

func putSwitch(ctx context.Context, h approvals.Handlers) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPut, "/autonomy/installation", strings.NewReader(`{"enabled":false}`)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.UpdateAutoApplyInstallation(rec, req)
	return rec
}

func switchHandlers(t *testing.T, e *Env, environ ...string) approvals.Handlers {
	t.Helper()
	t.Cleanup(func() { e.WsExec(t, `DELETE FROM setting WHERE key = $1`, approvals.AutoApplyEnabled.Key()) })
	cfg := configtest.StoredOver(t, e.Pool, approvals.ConfigSettings(), environ...)
	locks := func(surface []config.Any) *[]crmcontracts.SettingLock {
		l := compose.Locks(cfg, surface)
		return &l
	}
	return approvals.NewHandlers(approvals.NewService(e.DB())).WithSettings(cfg).WithLocks(locks)
}

func TestTheSwitchRouteTakesAnAdminAndRefusesEveryoneElse(t *testing.T) {
	e := Setup(t)
	h := switchHandlers(t, e)

	if rec := putSwitch(e.As(e.Rep1, nil, RepPerms), h); rec.Code != http.StatusForbidden {
		t.Errorf("a rep switching it off got %d, want 403", rec.Code)
	}
	agent := principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalAgent, ID: "agent:test", UserID: e.AdminUser, OnBehalfOf: e.AdminUser,
		PassportID: ids.NewV7(),
		Permissions: principal.Permissions{
			RoleKeys: []string{"admin"}, RowScope: principal.RowScopeAll,
			Objects: map[string]principal.ObjectGrant{"installation_settings": {Read: true, Update: true}},
		},
	})
	if rec := putSwitch(agent, h); rec.Code != http.StatusForbidden {
		t.Errorf("an agent switching it off got %d, want 403", rec.Code)
	}
	if !getAutonomy(e.As(e.Rep1, nil, RepPerms), h).On {
		t.Fatal("after two refusals the switch reads off, want still on")
	}
	if rec := putSwitch(e.AdminWithRole(), h); rec.Code != http.StatusOK {
		t.Fatalf("an admin switching it off got %d: %s", rec.Code, rec.Body.String())
	}
	if getAutonomy(e.As(e.Rep1, nil, RepPerms), h).On {
		t.Error("GET /autonomy still reports the switch on after the admin's PUT")
	}
}

func TestABootLockedSwitchAnswers409WithItsLocks(t *testing.T) {
	e := Setup(t)
	h := switchHandlers(t, e, "MARGINCE_APPROVALS_AUTO_APPLY_ENABLED=true")

	rec := putSwitch(e.AdminWithRole(), h)
	if rec.Code != http.StatusConflict {
		t.Fatalf("changing a boot-locked switch got %d, want 409", rec.Code)
	}
	got := getAutonomy(e.AdminWithRole(), h)
	if got.Locks == nil || len(got.Locks) != 1 || got.Locks[0].Key != approvals.AutoApplyEnabled.Key() {
		t.Fatalf("GET /autonomy locks = %+v, want the switch named as locked", got.Locks)
	}
}

type autonomyAnswer struct {
	On    bool                       `json:"auto_apply_enabled"`
	Locks []crmcontracts.SettingLock `json:"locks"`
}

func getAutonomy(ctx context.Context, h approvals.Handlers) autonomyAnswer {
	rec := httptest.NewRecorder()
	h.GetAutonomy(rec, httptest.NewRequest(http.MethodGet, "/autonomy", nil).WithContext(ctx))
	var out autonomyAnswer
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || rec.Code != http.StatusOK {
		panic("GET /autonomy answered " + rec.Body.String())
	}
	return out
}
