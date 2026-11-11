// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package meetingrecaps_test

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

func TestCalendarAgendaNeedsAuthoredNotesBeforeARecap(t *testing.T) {
	for _, provider := range []string{"gcal", "graphcal"} {
		t.Run(provider, func(t *testing.T) {
			e := integration.Setup(t)
			now := time.Date(2026, time.July, 15, 14, 0, 0, 0, time.UTC)
			contact, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{
				FullName: "Zora Buyer", Source: "manual",
				Emails: []contacts.ContactEmailInput{{Email: "buyer@customer.example", EmailType: "work", IsPrimary: true}},
			})
			if err != nil {
				t.Fatal(err)
			}
			const agenda = "<p>Agenda: discuss the account.</p>"
			ref, err := e.CalendarSink().Upsert(e.ConnectorOwnerCtx(e.AdminUser, provider), connector.NormalizedRecord{
				EntityType: "activity", NaturalKey: connector.NaturalKey{SourceSystem: provider, SourceID: "account-review"},
				Fields:       capture.ActivityFields{Kind: "meeting", Subject: "Account review", Body: agenda, OccurredAt: now.Add(-time.Hour)},
				Links:        []datasource.EntityRef{{Type: "contact", ID: ids.UUID(contact.Id)}},
				Participants: []connector.MessageParticipant{{Email: "buyer@customer.example", Role: "attendee"}},
				Source:       provider + ":account-review", CapturedBy: "connector:" + provider, Raw: []byte(`{}`),
			}.WithProviderAttestedParticipants(true))
			if err != nil {
				t.Fatal(err)
			}
			meeting, err := e.Activities.GetActivityContent(e.Admin(), ids.From[ids.ActivityKind](ref.ID), storekit.LiveOnly)
			if err != nil {
				t.Fatal(err)
			}
			if meeting.Version == nil {
				t.Fatal("captured meeting has no version")
			}
			ctx := principal.SystemActing(e.Admin(), "render-calendar-description")
			if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
				rendered, err := activities.RenderMeetingBodyTx(ctx, tx, activities.MeetingBodyRender{Activity: ref.ID, Version: *meeting.Version, StoredBody: agenda, Body: "Agenda: discuss the account."})
				if err == nil && !rendered {
					t.Error("calendar description was not rendered")
				}
				return err
			}); err != nil {
				t.Fatal(err)
			}
			store := activities.NewStore(e.DB()).WithClock(func() time.Time { return now })
			id := ids.From[ids.ActivityKind](ref.ID)
			if _, err := store.UpdateActivity(e.Admin(), id, activities.UpdateActivityInput{MeetingStatus: new("held")}); err != nil {
				t.Fatal(err)
			}
			if ready, err := store.ReadyForRecap(e.Admin(), id, now); ready || err != nil {
				t.Fatalf("invite-only recap readiness = %v, %v", ready, err)
			}
			if _, err := store.UpdateActivity(e.Admin(), id, activities.UpdateActivityInput{Body: new("We agreed to start the trial next week.")}); err != nil {
				t.Fatal(err)
			}
			if ready, err := store.ReadyForRecap(e.Admin(), id, now); !ready || err != nil {
				t.Fatalf("authored-note recap readiness = %v, %v", ready, err)
			}
		})
	}
}
