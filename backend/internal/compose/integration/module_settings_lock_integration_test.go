// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/modules/activities"
)

// lockedWrite is one request to a settings surface whose field a boot source
// sets, and the status it must answer.
type lockedWrite struct {
	name   string
	method string
	path   string
	body   any
	want   int
}

// expectLockedWrites sends each write in order and fails on any other status.
func expectLockedWrites(t *testing.T, e *apptest.AppEnv, writes []lockedWrite) {
	t.Helper()
	for _, w := range writes {
		if status := e.Call(t, w.method, w.path, w.body, nil, nil); status != w.want {
			t.Errorf("%s: %s %s → %d, want %d", w.name, w.method, w.path, status, w.want)
		}
	}
}

// The follow-up window the api's environment sets reads as that value and
// refuses a change, even an invalid one; resending it is accepted.
func TestFollowUpWindowSetByEnvIsLockedAgainstAChange(t *testing.T) {
	e := apptest.SetupAppWithEnv(t, []string{"MARGINCE_ACTIVITIES_FOLLOW_UP_AFTER_DAYS=6"})
	e.BootstrapWorkspace(t)

	const path = "/v1/activities/follow-up-settings"
	var read struct {
		Days int `json:"follow_up_after_days"`
	}
	if status := e.Call(t, http.MethodGet, path, nil, nil, &read); status != http.StatusOK || read.Days != 6 {
		t.Fatalf("GET %s → %d, %d days, want the 6 the environment sets", path, status, read.Days)
	}
	expectLockedWrites(t, e, []lockedWrite{
		{"a changed window", http.MethodPatch, path, map[string]any{"follow_up_after_days": 3}, http.StatusConflict},
		{"a window past its bounds", http.MethodPatch, path, map[string]any{"follow_up_after_days": 45}, http.StatusConflict},
		{"the locked window resent", http.MethodPatch, path, map[string]any{"follow_up_after_days": 6}, http.StatusOK},
	})
	if rows := settingRows(t, e, activities.FollowUpAfterDays.Key()); rows != 0 {
		t.Fatalf("the locked window wrote %d rows, want none", rows)
	}
}
