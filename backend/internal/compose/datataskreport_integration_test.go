// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
)

func TestCorrectionMetricsReadTheLedgerAndSurfaceTransportFailure(t *testing.T) {
	e := integration.Setup(t)
	ctx, engine, err := dataTaskJobEngine(t.Context(), e.Pool)
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.Seed(ctx); err != nil {
		t.Fatal(err)
	}
	section := dataTaskMetricsSection(e.Pool)
	var out bytes.Buffer
	if err := section(t.Context(), &out); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"margince_data_task_read_success 1", fmt.Sprintf(`margince_data_task_steps{status="pending"} %d`, registeredDataTaskSteps(dataTaskCatalog(e.Pool, time.Now)))} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("missing %q: %s", want, &out)
		}
	}
	refused := errors.New("connection reset")
	if err := section(t.Context(), &failingWriter{failOn: 0, err: refused}); !errors.Is(err, refused) {
		t.Fatalf("successful read hid a failed response: %v", err)
	}
	hideDataTaskLedger(t)
	out.Reset()
	if err := section(ctx, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "margince_data_task_read_success 0") || strings.Contains(out.String(), "margince_data_task_steps{") {
		t.Fatalf("failed ledger reported empty work: %s", &out)
	}
}

func TestCorrectionControlRejectsMalformedAndUnknownRequests(t *testing.T) {
	e := integration.Setup(t)
	h := dataTaskHandlers{pool: e.Pool}
	for _, tc := range []struct {
		body string
		want int
	}{
		{`{"task_id":`, http.StatusUnprocessableEntity},
		{`{"task_id":"core/0_unknown","step":"first","action":"pause","version":1}`, http.StatusNotFound},
	} {
		rec := httptest.NewRecorder()
		h.ControlDataTask(rec, httptest.NewRequest(http.MethodPost, "/v1/admin/data-tasks/control", strings.NewReader(tc.body)).WithContext(dataTaskReader(e)))
		if rec.Code != tc.want {
			t.Fatalf("control %q: %d %s", tc.body, rec.Code, rec.Body.String())
		}
	}
	rec := httptest.NewRecorder()
	h.ControlDataTask(rec, httptest.NewRequest(http.MethodPost, "/v1/admin/data-tasks/control", strings.NewReader(`{}`)))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("anonymous control=%d %s", rec.Code, rec.Body.String())
	}
}

func TestOwnerCorrectionCommandsRefuseInvalidActionsAndUnreadableLedger(t *testing.T) {
	e := integration.Setup(t)
	if _, err := DataTaskOperation(t.Context(), e.Pool, "execute"); err == nil {
		t.Fatal("unknown command accepted")
	}
	progress, err := DataTaskOperation(t.Context(), e.Pool, "estimate")
	if err != nil {
		t.Fatal(err)
	}
	if len(progress) != 3 || progress[0].Version != 0 || progress[0].Estimate != nil {
		t.Fatalf("unseeded estimate invented progress: %+v", progress)
	}
	hideDataTaskLedger(t)
	if _, err := DataTaskOperation(t.Context(), e.Pool, "status"); err == nil {
		t.Fatal("failed ledger reported a successful status")
	}
}

func hideDataTaskLedger(t *testing.T) {
	t.Helper()
	owner := integration.OwnerConn(t)
	if _, err := owner.Exec(t.Context(), `ALTER TABLE data_task_run RENAME TO data_task_run_unavailable_probe`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := owner.Exec(context.Background(), `ALTER TABLE data_task_run_unavailable_probe RENAME TO data_task_run`); err != nil {
			t.Errorf("restore correction ledger: %v", err)
		}
	})
}
