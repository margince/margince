// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package apptest

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// BookingCalendar is a connected, writable, empty provider calendar in front
// of the production seam, which still answers every record check.
type BookingCalendar struct {
	activities.SchedulingCalendar
}

func (*BookingCalendar) Check(context.Context, ids.UserID, string) error { return nil }

func (*BookingCalendar) List(context.Context, ids.UserID, string) ([]connector.CalendarOption, error) {
	return []connector.CalendarOption{{ID: "work", Name: "Work", Writable: true, Primary: true}}, nil
}

func (*BookingCalendar) Busy(context.Context, ids.UserID, string, string, time.Time, time.Time) ([]connector.CalendarInterval, error) {
	return []connector.CalendarInterval{}, nil
}

func (*BookingCalendar) Save(_ context.Context, _ ids.UserID, _ string, in connector.CalendarAppointment) (connector.CalendarReceipt, error) {
	return connector.CalendarReceipt{EventID: in.RequestID}, nil
}

// Option books an app's HTTP doors through this calendar.
func (c *BookingCalendar) Option() compose.Option {
	return compose.WithSchedulingCalendarForTest(func(seam activities.SchedulingCalendar) activities.SchedulingCalendar {
		c.SchedulingCalendar = seam
		return c
	})
}

// SetupBookingApp is SetupApp whose booking doors reach BookingCalendar, with
// the signed-in admin's booking page switched on.
func SetupBookingApp(t *testing.T) *AppEnv {
	t.Helper()
	e := SetupAppWithOptions(t, (&BookingCalendar{}).Option())
	e.BootstrapWorkspace(t)
	EnableBooking(t, e)
	return e
}

// EnableBooking switches on the signed-in caller's booking page.
func EnableBooking(t *testing.T, e *AppEnv) {
	t.Helper()
	if status := e.Call(t, http.MethodPut, "/v1/scheduling/profile", map[string]any{
		"enabled": true, "provider": "gcal", "calendar_id": "work", "duration_minutes": 30,
		"notice_minutes": 0, "horizon_days": 30, "buffer_minutes": 0, "title": "Meeting", "location": "",
	}, nil, nil); status != http.StatusOK {
		t.Fatalf("enabling the booking page → %d", status)
	}
}

// NextBookableMorning is 10:00 UTC on the next weekday at least a day ahead:
// inside the default working hours and the booking horizon.
func NextBookableMorning() time.Time {
	day := time.Now().UTC().AddDate(0, 0, 2)
	for day.Weekday() == time.Saturday || day.Weekday() == time.Sunday {
		day = day.AddDate(0, 0, 1)
	}
	return time.Date(day.Year(), day.Month(), day.Day(), 10, 0, 0, 0, time.UTC)
}
