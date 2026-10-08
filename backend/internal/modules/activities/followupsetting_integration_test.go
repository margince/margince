// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/settings"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func (e *loadEnv) settingsAdmin(update bool) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.ws)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.rep.String(), UserID: e.rep,
		Permissions: principal.Permissions{
			Objects: map[string]principal.ObjectGrant{
				"installation_settings": {Read: true, Update: update},
				"activity":              {Read: true},
			},
			RowScope: principal.RowScopeAll,
		},
	})
}

func followUpCall(ctx context.Context, t *testing.T, h Handlers, method, body string) (int, int) {
	t.Helper()
	req := httptest.NewRequest(method, "/v1/activities/follow-up-settings", strings.NewReader(body)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	if method == http.MethodGet {
		h.GetFollowUpSettings(rec, req)
	} else {
		h.UpdateFollowUpSettings(rec, req)
	}
	var out crmcontracts.FollowUpSettings
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("decoding the settings: %v", err)
		}
	}
	return rec.Code, out.FollowUpAfterDays
}

func TestAnAdminSetsTheFollowUpWindowTheWorklistApplies(t *testing.T) {
	e := setupLoad(t)
	db := database.BindTo(e.pool, ids.From[ids.WorkspaceKind](e.ws))
	h := NewHandlers(db).WithSettings(settings.New(e.pool, settings.NewRegistry(FollowUpAfterDays)))
	admin := e.settingsAdmin(true)

	if code, days := followUpCall(admin, t, h, http.MethodGet, ""); code != http.StatusOK || days != 2 {
		t.Fatalf("GET = %d, %d days; want 200 and the default 2", code, days)
	}
	if code, days := followUpCall(admin, t, h, http.MethodPatch, `{"follow_up_after_days": 5}`); code != http.StatusOK || days != 5 {
		t.Fatalf("PATCH 5 = %d, %d days; want 200 and 5", code, days)
	}
	_, window, err := NewStore(db).AwaitingReplies(admin, time.Now())
	if err != nil || window != 5 {
		t.Fatalf("the worklist read a %d-day window (%v), want the 5 the admin set", window, err)
	}
	if code, _ := followUpCall(admin, t, h, http.MethodPatch, `{"follow_up_after_days": 45}`); code != http.StatusUnprocessableEntity {
		t.Errorf("PATCH 45 = %d, want 422: the window is 1..30 days", code)
	}
	if code, _ := followUpCall(e.settingsAdmin(false), t, h, http.MethodPatch, `{"follow_up_after_days": 3}`); code != http.StatusForbidden {
		t.Errorf("PATCH by a seat without update = %d, want 403", code)
	}
}
