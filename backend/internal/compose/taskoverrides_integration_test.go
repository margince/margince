// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// Saving a task's thinking level and timeouts: the ETag a save is held to, the
// audit row, the preview that writes nothing, and who may do either.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func taskOverrideHandlers(e *integration.Env) aiRoutingHandlers {
	return aiRoutingHandlers{store: ai.NewRoutingStore(NewSettingsStore(e.Pool), config.Static(nil))}
}

func overrideSeat(e *integration.Env, update bool) context.Context {
	return e.As(e.AdminUser, nil, principal.Permissions{
		Objects:  map[string]principal.ObjectGrant{"ai_routing": {Read: true, Update: update}, "ai_budget": {Read: true}, "ai_diagnostics": {Read: true}},
		RowScope: principal.RowScopeAll,
	})
}

func putTaskOverrides(ctx context.Context, h aiRoutingHandlers, body, ifMatch string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPut, "/v1/ai/task-overrides", strings.NewReader(body)).WithContext(ctx)
	var params crmcontracts.ReplaceAiTaskOverridesParams
	if ifMatch != "" {
		params.IfMatch = &ifMatch
	}
	rec := httptest.NewRecorder()
	h.ReplaceAiTaskOverrides(rec, req, params)
	return rec
}

func getTaskOverrides(ctx context.Context, h aiRoutingHandlers) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.GetAiTaskOverrides(rec, httptest.NewRequest(http.MethodGet, "/v1/ai/task-overrides", nil).WithContext(ctx))
	return rec
}

func countAudit(t *testing.T, e *integration.Env) int {
	t.Helper()
	var n int
	if err := e.Pool.QueryRow(context.Background(), `SELECT count(*) FROM audit_log`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestTaskOverridesSaveUnderTheirOwnETagAndAuditTheWrite(t *testing.T) {
	e := integration.Setup(t)
	ctx := overrideSeat(e, true)
	h := taskOverrideHandlers(e)
	first := getTaskOverrides(ctx, h)
	if first.Code != http.StatusOK || strings.TrimSpace(first.Body.String()) != "{}" {
		t.Fatalf("GET of an installation with no overrides = %d %s", first.Code, first.Body)
	}
	audited := countAudit(t, e)

	put := putTaskOverrides(ctx, h, `{"capture_confidentiality_verdict":{"decision_timeout_ms":30000,"thinking":"low"}}`, first.Header().Get("ETag"))

	if put.Code != http.StatusOK {
		t.Fatalf("PUT = %d %s", put.Code, put.Body)
	}
	if n := countAudit(t, e) - audited; n != 1 {
		t.Fatalf("the save wrote %d audit rows, want 1", n)
	}
	again := getTaskOverrides(ctx, h)
	if again.Header().Get("ETag") != put.Header().Get("ETag") || !strings.Contains(again.Body.String(), `"decision_timeout_ms":30000`) {
		t.Fatalf("GET after the save = %s %s", again.Header().Get("ETag"), again.Body)
	}
	stale := putTaskOverrides(ctx, h, `{}`, first.Header().Get("ETag"))
	if stale.Code != http.StatusConflict {
		t.Fatalf("a save on the superseded ETag = %d %s, want 409", stale.Code, stale.Body)
	}
}

func TestATaskOverrideOutOfBoundsIsRefusedByItsPath(t *testing.T) {
	e := integration.Setup(t)
	put := putTaskOverrides(overrideSeat(e, true), taskOverrideHandlers(e), `{"capture_classify":{"decision_timeout_ms":15000,"attempt_timeout_ms":400000}}`, "")
	if put.Code != http.StatusUnprocessableEntity ||
		!strings.Contains(put.Body.String(), "capture_classify.decision_timeout_ms") ||
		!strings.Contains(put.Body.String(), "capture_classify.attempt_timeout_ms") {
		t.Fatalf("PUT = %d %s", put.Code, put.Body)
	}
}

func TestTheTaskOverridePreviewWritesNothingAndNamesStaleTasks(t *testing.T) {
	e := integration.Setup(t)
	ctx := overrideSeat(e, true)
	h := taskOverrideHandlers(e)
	audited := countAudit(t, e)
	rec := httptest.NewRecorder()

	h.PreviewAiTaskOverrides(rec, httptest.NewRequest(http.MethodPost, "/v1/ai/task-overrides/preview",
		strings.NewReader(`{"retired_task":{"thinking":"low"},"cold_start":{"attempt_timeout_ms":60000}}`)).WithContext(ctx))

	if rec.Code != http.StatusOK {
		t.Fatalf("preview = %d %s", rec.Code, rec.Body)
	}
	var got crmcontracts.AiTaskOverridesPreview
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Stale) != 1 || got.Stale[0] != "retired_task" || got.Errors == nil || (*got.Errors)[0].Field != "retired_task" {
		t.Fatalf("preview = %+v", got)
	}
	if got.Effective["cold_start"].AttemptTimeoutMs != 60000 || got.Effective["site_triage"].AttemptTimeoutMs != int(ai.CallCeiling.Milliseconds()) {
		t.Fatalf("effective = %+v", got.Effective)
	}
	if countAudit(t, e) != audited {
		t.Fatal("the preview wrote an audit row")
	}
}

func TestOnlyAHumanRoutingEditorMaySaveTaskOverrides(t *testing.T) {
	e := integration.Setup(t)
	h := taskOverrideHandlers(e)
	if rec := putTaskOverrides(overrideSeat(e, false), h, `{}`, ""); rec.Code != http.StatusForbidden {
		t.Fatalf("a read-only seat saved: %d %s", rec.Code, rec.Body)
	}
	agent := principal.WithActor(principal.WithWorkspaceID(context.Background(), e.WS), principal.Principal{
		Type: principal.PrincipalAgent, ID: "agent:task-overrides", SeatType: principal.SeatFull,
		Permissions: principal.Permissions{Objects: map[string]principal.ObjectGrant{"ai_routing": {Read: true, Update: true}}, RowScope: principal.RowScopeAll},
	})
	if rec := putTaskOverrides(agent, h, `{}`, ""); rec.Code != http.StatusForbidden {
		t.Fatalf("an agent saved: %d %s", rec.Code, rec.Body)
	}
	if rec := getTaskOverrides(agent, h); rec.Code != http.StatusForbidden {
		t.Fatalf("an agent read: %d %s", rec.Code, rec.Body)
	}
}

func TestTheStatusMarksATaskWithAnOverride(t *testing.T) {
	e := integration.Setup(t)
	ctx := overrideSeat(e, true)
	if rec := putTaskOverrides(ctx, taskOverrideHandlers(e), `{"cold_start":{"thinking":"high"}}`, ""); rec.Code != http.StatusOK {
		t.Fatalf("PUT = %d %s", rec.Code, rec.Body)
	}
	store := ai.NewAdminStore(e.DB(), NewSettingsStore(e.Pool), budgetFullUsers, aiDeferredWork(e.Pool))

	status, err := store.ReadStatus(ctx)

	if err != nil {
		t.Fatal(err)
	}
	for _, row := range status.Features {
		if row.Task != "cold_start" {
			continue
		}
		if row.Overrides == nil || row.Overrides.Thinking == nil || *row.Overrides.Thinking != "high" || row.Defaults == nil {
			t.Fatalf("cold_start row = %+v", row)
		}
		return
	}
	t.Fatal("no cold_start row in the status")
}
