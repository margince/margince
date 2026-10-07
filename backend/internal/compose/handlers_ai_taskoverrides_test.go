// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Who may read or save the task overrides. Every refusal here happens before a
// query: the store is built over no pool, so a check that ran after one would
// fail on the nil pool rather than with the 403 asserted.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func putTaskOverrides(ctx context.Context, h aiRoutingHandlers, body, ifMatch string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPut, "/v1/ai/task-overrides", strings.NewReader(body)).WithContext(ctx)
	if ifMatch != "" {
		req.Header.Set("If-Match", ifMatch)
	}
	rec := httptest.NewRecorder()
	h.ReplaceAiTaskOverrides(rec, req)
	return rec
}

func getTaskOverrides(ctx context.Context, h aiRoutingHandlers) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.GetAiTaskOverrides(rec, httptest.NewRequest(http.MethodGet, "/v1/ai/task-overrides", nil).WithContext(ctx))
	return rec
}

func overrideActor(kind principal.PrincipalType, grant principal.ObjectGrant) context.Context {
	return principal.WithActor(principal.WithWorkspaceID(context.Background(), ids.NewV7()), principal.Principal{
		Type: kind, ID: "seat:task-overrides", SeatType: principal.SeatFull,
		Permissions: principal.Permissions{Objects: map[string]principal.ObjectGrant{"ai_routing": grant}, RowScope: principal.RowScopeAll},
	})
}

func TestOnlyAHumanRoutingEditorMaySaveTaskOverrides(t *testing.T) {
	h := aiRoutingHandlers{store: ai.NewRoutingStore(NewSettingsStore(nil), config.Static(nil))}
	reader := overrideActor(principal.PrincipalHuman, principal.ObjectGrant{Read: true})
	if rec := putTaskOverrides(reader, h, `{}`, ""); rec.Code != http.StatusForbidden {
		t.Fatalf("a read-only seat saved: %d %s", rec.Code, rec.Body)
	}
	agent := overrideActor(principal.PrincipalAgent, principal.ObjectGrant{Read: true, Update: true})
	if rec := putTaskOverrides(agent, h, `{}`, ""); rec.Code != http.StatusForbidden {
		t.Fatalf("an agent saved: %d %s", rec.Code, rec.Body)
	}
	if rec := getTaskOverrides(agent, h); rec.Code != http.StatusForbidden {
		t.Fatalf("an agent read: %d %s", rec.Code, rec.Body)
	}
}
