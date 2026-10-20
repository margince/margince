// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/modules/capture"
)

// settingRows counts the saved rows of one setting key.
func settingRows(t *testing.T, e *apptest.AppEnv, key string) int {
	t.Helper()
	var rows int
	if err := e.Owner.QueryRow(context.Background(),
		`SELECT count(*) FROM setting WHERE key = $1`, key).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	return rows
}

// Capture settings the api's environment sets read as that value, refuse a
// change, and let a form that resends them save its other fields.
func TestCaptureSettingsSetByEnvAreLockedAgainstAChange(t *testing.T) {
	e := apptest.SetupAppWithEnv(t, []string{
		"MARGINCE_CAPTURE_AUTO_ENRICH=false",
		"MARGINCE_CAPTURE_SITE_READ_MAX_PAGES=10",
	})
	e.BootstrapWorkspace(t)

	var read struct {
		AutoEnrich bool `json:"auto_enrich"`
		SiteRead   struct {
			MaxPages int `json:"max_pages"`
			MaxMib   int `json:"max_mib"`
		} `json:"site_read"`
	}
	if status := e.Call(t, http.MethodGet, "/v1/capture/settings", nil, nil, &read); status != http.StatusOK {
		t.Fatalf("GET /capture/settings → %d", status)
	}
	if read.AutoEnrich || read.SiteRead.MaxPages != 10 {
		t.Fatalf("reads auto_enrich %v and max_pages %d, want the false and 10 the environment sets",
			read.AutoEnrich, read.SiteRead.MaxPages)
	}
	for _, change := range []map[string]any{{"auto_enrich": true}, {"site_read_max_pages": 20}} {
		if status := e.Call(t, http.MethodPatch, "/v1/capture/settings", change, nil, nil); status != http.StatusConflict {
			t.Fatalf("a change of a locked setting %v → %d, want 409", change, status)
		}
	}
	if status := e.Call(t, http.MethodPatch, "/v1/capture/settings",
		map[string]any{"auto_enrich": false, "site_read_max_pages": 10, "site_read_max_mib": 16}, nil, &read); status != http.StatusOK {
		t.Fatalf("a form resending the locked values beside a change → %d, want 200", status)
	}
	if read.SiteRead.MaxMib != 16 {
		t.Fatalf("after the save max_mib reads %d, want the 16 saved", read.SiteRead.MaxMib)
	}
	for _, key := range []string{capture.AutoEnrich.Key(), capture.SiteReadMaxPages.Key()} {
		if rows := settingRows(t, e, key); rows != 0 {
			t.Fatalf("the locked %s wrote %d rows, want none", key, rows)
		}
	}
}
