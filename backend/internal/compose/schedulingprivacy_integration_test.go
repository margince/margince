// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

type privateCalendar struct{ activities.SchedulingCalendar }

func (privateCalendar) Check(context.Context, ids.UserID, string) error { return nil }
func (privateCalendar) CheckRecipient(context.Context, ids.UserID, ids.UUID, string) error {
	return nil
}

func (privateCalendar) List(context.Context, ids.UserID, string) ([]connector.CalendarOption, error) {
	return []connector.CalendarOption{{ID: "primary", Primary: true, Writable: true}}, nil
}

func (privateCalendar) Busy(context.Context, ids.UserID, string, string, time.Time, time.Time) ([]connector.CalendarInterval, error) {
	return []connector.CalendarInterval{}, nil
}

func TestMeetingCapabilitiesAreExportedSafelyAndErased(t *testing.T) {
	for _, anonymize := range []bool{false, true} {
		name := "erasure"
		if anonymize {
			name = "anonymization"
		}
		t.Run(name, func(t *testing.T) { verifyMeetingPrivacy(t, anonymize) })
	}
}

func verifyMeetingPrivacy(t *testing.T, anonymize bool) {
	t.Helper()
	e := integration.Setup(t)
	ctx := e.Admin()
	contact := e.SeedContact(t, "Booking guest", nil)
	vault := keyvault.NewMemory()
	now := time.Date(2026, 10, 5, 6, 0, 0, 0, time.UTC)
	store := e.Activities.WithClock(func() time.Time { return now }).WithPublicBaseURL("https://crm.example.test").WithMeetingVault(vault).WithSchedulingCalendar(privateCalendar{}).WithWorkingHours(func(context.Context, ids.UserID) (activities.WorkingHours, error) {
		return activities.WorkingHours{StartMinute: 540, EndMinute: 1020, Days: []int{1, 2, 3, 4, 5}, Location: time.UTC}, nil
	})
	if _, err := store.SaveSchedulingProfile(ctx, crmcontracts.SchedulingProfile{Provider: "gcal", CalendarId: "primary", DurationMinutes: 30, NoticeMinutes: 120, HorizonDays: 30, Title: "Discovery"}); err != nil {
		t.Fatal(err)
	}
	request := crmcontracts.MeetingInvitationRequest{ContactId: crmcontracts.Id(contact), AttendeeEmail: "guest@example.test", Subject: "Private project discovery", Start: now.Add(4 * time.Hour), End: now.Add(5 * time.Hour)}
	meeting, err := store.CreateInvitation(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	proposal, err := store.CreateProposal(ctx, crmcontracts.MeetingProposalRequest{ContactId: request.ContactId, AttendeeEmail: request.AttendeeEmail, Subject: request.Subject, DurationMinutes: 30})
	if err != nil {
		t.Fatal(err)
	}
	pkg, err := privacy.AssembleSAR(ctx, e.DB(), ids.From[ids.ContactKind](contact))
	if err != nil {
		t.Fatal(err)
	}
	if len(pkg.MeetingInvitations) != 1 || len(pkg.MeetingProposals) != 1 {
		t.Fatalf("missing meeting sections: %d %d", len(pkg.MeetingInvitations), len(pkg.MeetingProposals))
	}
	encoded, err := json.Marshal(pkg.MeetingInvitations)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{*meeting.ManagementToken, "management_ref", "management_hash", "PassportID", "RequestID", "CalendarID"} {
		if strings.Contains(string(encoded), secret) {
			t.Errorf("export contains private authority %q", secret)
		}
	}
	if anonymize {
		service := NewRetentionServiceFor(e.DB(), nil, slog.Default()).WithPayloadVault(controllerPayloads{v: vault})
		if _, err := service.AnonymiseContacts(ctx, []ids.UUID{contact}, privacy.PurgeOwnerRule); err != nil {
			t.Fatal(err)
		}
	} else {
		eraser := privacy.NewEraser(e.DB()).WithPayloadVault(controllerPayloads{v: vault})
		if err := eraser.EraseContact(ctx, contact, "subject request"); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := store.ResolveMeetingToken(ctx, *meeting.ManagementToken); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("meeting capability survived erasure: %v", err)
	}
	token := strings.Split(proposal.URL, "proposal-")[1]
	if _, _, err := store.ResolveProposalToken(ctx, token); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("proposal survived erasure: %v", err)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM meeting_invitation WHERE status<>'erased' OR management_ref<>'' OR appointment ? 'Attendees'`); got != 0 {
		t.Fatalf("pending delivery survived erasure: %d", got)
	}
}

func TestInvitationRecipientMustBelongToAVisibleContact(t *testing.T) {
	e := integration.Setup(t)
	contact := e.SeedContact(t, "Booking guest", nil)
	address := "guest@example.test"
	e.WsExec(t, `INSERT INTO contact_email(contact_id,email,is_primary,source,captured_by) VALUES($1,$2,true,'manual','human:test')`, contact, address)
	e.WsExec(t, `INSERT INTO role(key,name,permissions) VALUES('calendar_host','Calendar host','{"objects":{"contact":{"read":true}},"row_scope":"all"}')`)
	e.WsExec(t, `INSERT INTO role_assignment(role_id,user_id) SELECT id,@host FROM role WHERE key='calendar_host'`, pgx.NamedArgs{"host": e.AdminUser})
	calendar := newSchedulingCalendar(e.Pool, nil)
	host := ids.From[ids.UserKind](e.AdminUser)
	if err := calendar.CheckRecipient(e.Admin(), host, contact, address); err != nil {
		t.Fatalf("existing visible contact rejected: %v", err)
	}
	if err := calendar.CheckRecipient(e.Admin(), host, contact, "other@example.test"); err == nil {
		t.Fatal("arbitrary address accepted for a different contact")
	}
}
