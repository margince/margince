// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"net/http"
	"strings"
	"testing"
)

// Saving the meeting settings refuses each mistake on the field it is about:
// the screen marks that field instead of showing one generic sentence.
func TestMeetingSettingsRefuseEachMistakeOnItsOwnField(t *testing.T) {
	e := setupBookingApp(t)
	e.BootstrapWorkspace(t)
	enableBookingPage(t, e)
	valid := func() AnyMap {
		return AnyMap{
			"provider": "gcal", "calendar_id": "ada@example.com", "enabled": false, "duration_minutes": 30,
			"notice_minutes": 0, "buffer_minutes": 10, "horizon_days": 30, "title": "Meeting", "location": "Video call",
		}
	}
	for name, tc := range map[string]struct {
		change func(AnyMap)
		field  string
	}{
		"null buffer":      {func(b AnyMap) { b["buffer_minutes"] = nil }, "buffer_minutes"},
		"missing notice":   {func(b AnyMap) { delete(b, "notice_minutes") }, "notice_minutes"},
		"buffer too large": {func(b AnyMap) { b["buffer_minutes"] = 121 }, "buffer_minutes"},
		"duration too low": {func(b AnyMap) { b["duration_minutes"] = 5 }, "duration_minutes"},
		"title too long":   {func(b AnyMap) { b["title"] = strings.Repeat("t", 255) }, "title"},
		"calendar id long": {func(b AnyMap) { b["calendar_id"] = strings.Repeat("c", 513) }, "calendar_id"},
	} {
		t.Run(name, func(t *testing.T) {
			body := valid()
			tc.change(body)
			var problem struct {
				Details struct {
					Errors []struct {
						Field string `json:"field"`
					} `json:"errors"`
				} `json:"details"`
			}
			if status := e.Call(t, "PUT", "/v1/scheduling/profile", body, nil, &problem); status != http.StatusUnprocessableEntity {
				t.Fatalf("status = %d, want 422", status)
			}
			if len(problem.Details.Errors) != 1 || problem.Details.Errors[0].Field != tc.field {
				t.Fatalf("errors = %+v, want one error on %s", problem.Details.Errors, tc.field)
			}
		})
	}

	if status := e.Call(t, "PUT", "/v1/scheduling/profile", valid(), nil, nil); status != http.StatusOK {
		t.Fatalf("a valid profile saved with status %d, want 200", status)
	}
}
