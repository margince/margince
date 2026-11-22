// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// A host with no calendar provider asks for reliable availability and is told
// to connect a calendar, on the route and in the tool alike.
func TestReliableAvailabilityWithoutACalendarAsksForOneAtBothDoors(t *testing.T) {
	d := newTwoDoorsWith(t, "availability-no-calendar", []compose.Option{(&apptest.BookingCalendar{}).Option()}, "read")
	day := apptest.NextBookableMorning(t).Truncate(24 * time.Hour)
	from, to := day.Add(8*time.Hour).Format(time.RFC3339), day.Add(18*time.Hour).Format(time.RFC3339)

	var body map[string]any
	if status := d.e.Call(t, http.MethodGet, "/v1/availability?reliable=true&from="+from+"&to="+to, nil, d.bearer, &body); status != http.StatusUnprocessableEntity {
		t.Fatalf("the route answered %d %v, want 422", status, body)
	}
	refusal := d.mcp.CallRefused(t, "check_availability", map[string]any{"from": from, "to": to, "reliable": true})
	if !strings.Contains(refusal, "Connect a calendar") || strings.Contains(refusal, "permission denied") {
		t.Errorf("the tool answered %q, want the same ask to connect a calendar", refusal)
	}
}
