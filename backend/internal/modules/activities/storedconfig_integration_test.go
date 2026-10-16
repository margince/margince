// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"io/fs"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/settings"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// followUpConfig is this module's configuration as a program loads it from
// environ alone, reading and saving through pool.
func followUpConfig(t *testing.T, pool *pgxpool.Pool, environ ...string) *config.Config {
	t.Helper()
	reg, err := config.NewRegistry(ConfigSettings())
	if err != nil {
		t.Fatalf("the activities settings do not register: %v", err)
	}
	cfg, err := config.Load(reg, config.Sources{
		Environ:  environ,
		ReadFile: func(string) ([]byte, error) { return nil, fs.ErrNotExist },
	})
	if err != nil {
		t.Fatalf("loading the activities settings from %v: %v", environ, err)
	}
	return cfg.WithDatabase(settings.New(pool, settings.NewRegistry(cfg.StoredDefinitions()...)))
}

// A window the environment sets is the one the worklist applies, over a saved
// row, and the endpoint refuses any other value before it judges it. Not
// parallel: the binding is the process's own.
func TestABootSetFollowUpWindowBindsTheWorklistAndRefusesAChange(t *testing.T) {
	e := setupLoad(t)
	e.restoreFollowUpWindow(t)
	db := database.BindTo(e.pool, ids.From[ids.WorkspaceKind](e.ws))
	admin := e.settingsAdmin(true)
	if code, _ := followUpCall(admin, t, NewHandlers(db).WithSettings(followUpConfig(t, e.pool)),
		http.MethodPatch, `{"follow_up_after_days": 9}`); code != http.StatusOK {
		t.Fatalf("saving 9 days with nothing set at boot = %d, want 200", code)
	}
	locked := followUpConfig(t, e.pool, "MARGINCE_ACTIVITIES_FOLLOW_UP_AFTER_DAYS=4")
	before := config.Bound()
	config.Bind(locked)
	t.Cleanup(func() { config.Bind(before) })

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
