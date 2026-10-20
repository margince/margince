// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func dataTaskReader(e *integration.Env) context.Context {
	return e.As(e.AdminUser, nil, principal.Permissions{Objects: map[string]principal.ObjectGrant{"data_task": {Read: true, Update: true}}, RowScope: principal.RowScopeAll})
}

func TestDataTaskHTTPControlsRequireCurrentHumanAuthority(t *testing.T) {
	e := integration.Setup(t)
	h := dataTaskHandlers{pool: e.Pool}
	ctx := dataTaskReader(e)
	agent := principal.WithActor(ctx, principal.Principal{Type: principal.PrincipalAgent, ID: "agent:test", UserID: e.AdminUser, Permissions: principal.Permissions{Objects: map[string]principal.ObjectGrant{"data_task": {Read: true, Update: true}}}})
	for _, denied := range []context.Context{t.Context(), e.As(e.Rep1, nil, principal.Permissions{}), agent} {
		rec := httptest.NewRecorder()
		h.ListDataTasks(rec, httptest.NewRequest(http.MethodGet, "/v1/admin/data-tasks", nil).WithContext(denied))
		if rec.Code != http.StatusForbidden {
			t.Fatalf("denied reader=%d %s", rec.Code, rec.Body.String())
		}
	}
	rec := httptest.NewRecorder()
	h.ListDataTasks(rec, httptest.NewRequest(http.MethodGet, "/v1/admin/data-tasks", nil).WithContext(ctx))
	var unseeded struct {
		Data []crmcontracts.DataTaskProgress `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &unseeded); err != nil {
		t.Fatal(err)
	}
	if rec.Code != 200 || len(unseeded.Data) != 2 || unseeded.Data[0].Estimate != nil || unseeded.Data[0].Version != 0 {
		t.Fatalf("unseeded=%s", rec.Body.String())
	}
	engine, err := newDataTaskEngine(e.Pool, "test", func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) })
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.Seed(principal.SystemActing(ctx, "system:test")); err != nil {
		t.Fatal(err)
	}
	progress, err := engine.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	p := progress[0]
	body, err := json.Marshal(crmcontracts.DataTaskControl{TaskId: p.TaskID, Step: p.Step, Action: "pause", Version: p.Version})
	if err != nil {
		t.Fatal(err)
	}
	for i, want := range []int{204, 409} {
		rec := httptest.NewRecorder()
		h.ControlDataTask(rec, httptest.NewRequest(http.MethodPost, "/v1/admin/data-tasks/control", strings.NewReader(string(body))).WithContext(ctx))
		if rec.Code != want {
			t.Fatalf("control%d=%d want%d: %s", i, rec.Code, want, rec.Body.String())
		}
	}
	progress, err = engine.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if progress[0].Status != datatask.Paused || progress[1].BlockedReason != "earlier_step" {
		t.Fatalf("paused ordering=%+v", progress)
	}
}

func TestDataTaskReadFailureNeverReturnsEmptyProgress(t *testing.T) {
	e := integration.Setup(t)
	pool, err := testdb.OwnPool(t.Context(), os.Getenv("MARGINCE_TEST_APP_DSN"))
	if err != nil {
		t.Fatal(err)
	}
	pool.Close()
	rec := httptest.NewRecorder()
	dataTaskHandlers{pool: pool}.ListDataTasks(rec, httptest.NewRequest(http.MethodGet, "/v1/admin/data-tasks", nil).WithContext(dataTaskReader(e)))
	if rec.Code != http.StatusInternalServerError || strings.Contains(rec.Body.String(), `"data":[]`) {
		t.Fatalf("failed ledger=%d %s", rec.Code, rec.Body.String())
	}
}

func TestCorrectionFailureNoticeUsesCurrentOperatorGrants(t *testing.T) {
	e := integration.Setup(t)
	ctx := principal.SystemActing(e.Admin(), "system:test")
	e.WsExec(t, `UPDATE role SET permissions=jsonb_set(permissions,'{objects,data_task}','{"read":false,"create":false,"update":false,"delete":false}'::jsonb) WHERE key='ops'`)
	e.WsExec(t, `INSERT INTO role_assignment(user_id,role_id) SELECT @admin::uuid,id FROM role WHERE key='admin' UNION ALL SELECT @ops::uuid,id FROM role WHERE key='ops' ON CONFLICT DO NOTHING`, pgx.NamedArgs{"admin": e.AdminUser, "ops": e.Rep1})
	p := datatask.Progress{TaskID: "core/100_example", Step: "first", RunID: ids.NewV7(), Version: 5}
	write := dataTaskFailureNotice(e.Pool)
	for range 2 {
		if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error { return write(ctx, tx, p) }); err != nil {
			t.Fatal(err)
		}
	}
	var count int
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM notice WHERE kind='data_task_failed' AND recipient_user_id=@user`, pgx.NamedArgs{"user": e.AdminUser}).Scan(&count)
	}); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("operator notice count=%d", count)
	}
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM notice WHERE kind='data_task_failed' AND recipient_user_id=@user`, pgx.NamedArgs{"user": e.Rep1}).Scan(&count)
	}); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("revoked operator received a notice")
	}
}

func TestResumingCorrectionQueuesReconciliationImmediately(t *testing.T) {
	e := integration.Setup(t)
	integration.ApplyRiverSchema(t)
	ctx := dataTaskReader(e)
	engine, err := newDataTaskEngine(e.Pool, "test", func() time.Time { return clocktest.Now(t) })
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.Seed(principal.SystemActing(ctx, "system:test")); err != nil {
		t.Fatal(err)
	}
	e.WsExec(t, `UPDATE data_task_run SET status='paused' WHERE step='stamp_provider_received_at'`)
	progress, err := engine.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	p := progress[0]
	body, err := json.Marshal(crmcontracts.DataTaskControl{TaskId: p.TaskID, Step: p.Step, Action: "resume", Version: p.Version})
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	dataTaskHandlers{pool: e.Pool}.ControlDataTask(rec, httptest.NewRequest(http.MethodPost, "/v1/admin/data-tasks/control", strings.NewReader(string(body))).WithContext(ctx))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("resume=%d %s", rec.Code, rec.Body.String())
	}
	if got := e.WsCount(t, `SELECT count(*) FROM river_job WHERE kind='data_task_reconcile'`); got != 1 {
		t.Fatalf("resume jobs=%d", got)
	}
}
