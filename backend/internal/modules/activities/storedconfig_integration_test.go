// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/config/configtest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A window the environment sets is the one the worklist applies, over a saved
// row. The endpoint refuses any other value before it judges it. Not parallel:
// the binding is the process's own.
func TestABootSetFollowUpWindowBindsTheWorklistAndRefusesAChange(t *testing.T) {
	e := setupLoad(t)
	e.restoreFollowUpWindow(t)
	db := database.BindTo(e.pool, ids.From[ids.WorkspaceKind](e.ws))
	admin := e.settingsAdmin(true)
	if code, _ := followUpCall(admin, t, NewHandlers(db).WithSettings(configtest.StoredOver(t, e.pool, ConfigSettings())),
		http.MethodPatch, `{"follow_up_after_days": 9}`); code != http.StatusOK {
		t.Fatalf("saving 9 days with nothing set at boot = %d, want 200", code)
	}
	locked := configtest.StoredOver(t, e.pool, ConfigSettings(), "MARGINCE_ACTIVITIES_FOLLOW_UP_AFTER_DAYS=4")
	configtest.Bind(t, locked)

	_, window, err := NewStore(db).AwaitingReplies(admin, clocktest.Now(t))
	if err != nil || window != 4 {
		t.Fatalf("the worklist read a %d-day window (%v), want the 4 the environment sets over the saved 9", window, err)
	}
	h := NewHandlers(db).WithSettings(locked)
	for body, want := range map[string]int{
		`{"follow_up_after_days": 9}`:  http.StatusConflict,
		`{"follow_up_after_days": 45}`: http.StatusConflict,
		`{"follow_up_after_days": 4}`:  http.StatusOK,
	} {
		if code, _ := followUpCall(admin, t, h, http.MethodPatch, body); code != want {
			t.Errorf("PATCH %s under a boot-set window = %d, want %d", body, code, want)
		}
	}
	if code, _ := followUpCall(e.settingsAdmin(false), t, h, http.MethodPatch, `{"follow_up_after_days": 9}`); code != http.StatusForbidden {
		t.Errorf("PATCH by a seat without update under a boot-set window = %d, want 403", code)
	}
}
