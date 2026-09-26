// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

type erasingCalendar struct {
	privateCalendar
	erase    func() error
	canceled string
}

//nolint:nilnil // No earlier event exists at this provider boundary.
func (c *erasingCalendar) Lookup(context.Context, ids.UserID, string, connector.CalendarAppointment) (*connector.CalendarReceipt, error) {
	return nil, nil
}

func (c *erasingCalendar) Save(_ context.Context, _ ids.UserID, _ string, in connector.CalendarAppointment) (connector.CalendarReceipt, error) {
	if err := c.erase(); err != nil {
		return connector.CalendarReceipt{}, err
	}
	return connector.CalendarReceipt{EventID: in.RequestID, UID: "private@calendar.test", URL: "https://calendar.example.test/private"}, nil
}

func (c *erasingCalendar) Cancel(_ context.Context, _ ids.UserID, _, _, event string) error {
	c.canceled = event
	return nil
}

func TestErasureDuringProviderDeliveryCancelsTheLateReceiptAndSuppressesItsEcho(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.Admin()
	contact := e.SeedContact(t, "Booking guest", nil)
	vault := keyvault.NewMemory()
	now := time.Date(2026, 10, 5, 6, 0, 0, 0, time.UTC)
	eraser := privacy.NewEraser(e.DB()).WithPayloadVault(controllerPayloads{v: vault})
	calendar := &erasingCalendar{erase: func() error { return eraser.EraseContact(ctx, contact, "subject request") }}
	store := e.Activities.WithClock(func() time.Time { return now }).WithPublicBaseURL("https://crm.example.test").WithMeetingVault(vault).WithSchedulingCalendar(calendar).WithWorkingHours(func(context.Context, ids.UserID) (activities.WorkingHours, error) {
		return activities.WorkingHours{StartMinute: 540, EndMinute: 1020, Days: []int{1, 2, 3, 4, 5}, Location: time.UTC}, nil
	})
	if _, err := store.SaveSchedulingProfile(ctx, crmcontracts.SchedulingProfile{Provider: "gcal", CalendarId: "primary", DurationMinutes: 30, NoticeMinutes: 120, HorizonDays: 30, Title: "Discovery"}); err != nil {
		t.Fatal(err)
	}
	meeting, err := store.CreateInvitation(ctx, crmcontracts.MeetingInvitationRequest{ContactId: crmcontracts.Id(contact), AttendeeEmail: "guest@example.test", Subject: "Private subject", Start: now.Add(4 * time.Hour), End: now.Add(5 * time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.DeliverInvitations(ctx); err != nil {
		t.Fatal(err)
	}
	now = now.Add(2 * time.Minute)
	if err := store.DeliverInvitations(ctx); err != nil {
		t.Fatal(err)
	}
	if calendar.canceled != ids.UUID(meeting.Id).String() {
		t.Fatalf("late provider event was not canceled: %q", calendar.canceled)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM meeting_invitation WHERE status='erased' AND cleanup_complete AND management_ref='' AND NOT appointment ? 'Attendees' AND NOT receipt ? 'url'`); got != 1 {
		t.Fatalf("erasure retained personal delivery material or missed cleanup: %d", got)
	}
	err = e.DB().Tx(ctx, func(tx pgx.Tx) error {
		_, _, err := activities.ResolveCalendarInvitation(ctx, tx, "gcal", calendar.canceled, ids.UUID(meeting.Id).String())
		return err
	})
	if !errors.Is(err, connector.ErrSkip) {
		t.Fatalf("erased provider echo can land again: %v", err)
	}
}
