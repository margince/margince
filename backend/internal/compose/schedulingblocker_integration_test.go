// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The blocker is read from the connection row, not from the provider. A
// read-only grant says so before any call. A missing connection reads as
// unavailable, not as a page that works.
func TestABookingCalendarsBlockerComesFromItsConnection(t *testing.T) {
	e := integration.Setup(t)
	e.GrantRole(t, e.Rep1, "rep")
	calendar := newSchedulingCalendar(e.Pool, captureHealthRegistry(e.DB()))
	host := ids.From[ids.UserKind](e.Rep1)
	blocker := func() crmcontracts.SchedulingProfileBlocker {
		t.Helper()
		got, err := calendar.Blocker(e.Admin(), host, "gcal")
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	if got := blocker(); got != crmcontracts.SchedulingProfileBlockerCalendarUnavailable {
		t.Errorf("no connection reads %q, want calendar_unavailable", got)
	}
	e.WsExec(t, `INSERT INTO capture_connection (provider, user_id, status, provider_scopes)
		VALUES ('gcal', $1, 'connected', ARRAY['https://www.googleapis.com/auth/calendar.readonly'])`, e.Rep1)
	if got := blocker(); got != crmcontracts.SchedulingProfileBlockerCalendarReadOnly {
		t.Errorf("a read-only grant reads %q, want calendar_read_only", got)
	}
}
