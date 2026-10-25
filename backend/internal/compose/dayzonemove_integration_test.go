// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/contact360"
	"github.com/margince/margince/backend/internal/compose/contactbrief"
	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/comms"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/relstrength"
)

// A day entry's date is its UTC date. Moving the installation zone moves
// nothing: not the stored instant, a brief's date, today's lane or a quiet count.
// The lane is read late in the evening east of UTC, early in the morning west.
func TestADayEntryKeepsItsDateWhenTheInstallationZoneMoves(t *testing.T) {
	anchor := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	for _, move := range []struct{ from, to string }{
		{"UTC", "Pacific/Auckland"},
		{"Asia/Tokyo", "America/Los_Angeles"},
	} {
		t.Run(move.from+" to "+move.to, func(t *testing.T) {
			e := integration.Setup(t)
			setInstallationZone(t, e, move.from)
			rep := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms)
			contact := seedLinkedContact(t, e, "fair@day-zone.example")
			named := time.Date(2026, 3, 10, 9, 0, 0, 0, time.UTC)
			subject := "Trade fair"
			meeting, _, err := e.Activities.LogActivity(rep, activities.LogActivityInput{
				Kind: "meeting", Subject: &subject, OccurredAt: &named, Source: "manual",
				OccurredPrecision: activities.PrecisionDay,
				Links:             []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}},
			})
			if err != nil {
				t.Fatalf("logging the day meeting: %v", err)
			}
			to := setInstallationZone(t, e, move.to)
			if stored := storedStart(t, e, ids.UUID(meeting.Id)); !stored.Equal(anchor) {
				t.Errorf("stored at %v after the zone moved, want %v", stored, anchor)
			}

			lateOnTheDate := time.Date(2026, 3, 10, 23, 30, 0, 0, to)
			if move.to == "America/Los_Angeles" {
				lateOnTheDate = time.Date(2026, 3, 10, 8, 0, 0, 0, to)
			}
			clock := func() time.Time { return lateOnTheDate }
			if when := briefWhen(t, e, contact, ids.UUID(meeting.Id), clock); when != "2026-03-10" {
				t.Errorf("the brief dates it %q, want 2026-03-10", when)
			}
			day := assembleFeed(rep, t, e, lateOnTheDate)
			if day.Meetings == nil || !slices.Contains(titlesOn(*day.Meetings), subject) {
				t.Errorf("at %v in %s today's meetings are %v, want the trade fair", lateOnTheDate, move.to, day.Meetings)
			}

			// Early on the 45th date after it, where the stored noon is already
			// the next local morning east of UTC.
			quietRead := time.Date(2026, 4, 24, 9, 0, 0, 0, to)
			if days := quietDays(t, e, contact, func() time.Time { return quietRead }); days != relstrength.QuietDays {
				t.Errorf("at %v in %s the contact reads quiet for %d days, want %d", quietRead, move.to, days, relstrength.QuietDays)
			}
		})
	}
}

// setInstallationZone moves the zone that says which date is today, through
// the installation settings an admin saves.
func setInstallationZone(t *testing.T, e *integration.Env, zone string) *time.Location {
	t.Helper()
	admin := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms)
	settings := identity.NewInstallationSettings(e.DB(), NewSettingsStore(e.Pool), StoredConfig(e.Pool))
	if _, err := settings.UpdateInstallation(admin, identity.InstallationPatch{Timezone: &zone}); err != nil {
		t.Fatalf("saving the installation zone %s: %v", zone, err)
	}
	loc, err := time.LoadLocation(zone)
	if err != nil {
		t.Fatal(err)
	}
	return loc
}

func storedStart(t *testing.T, e *integration.Env, id ids.UUID) time.Time {
	t.Helper()
	var at time.Time
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT occurred_at FROM activity WHERE id = $1`, id).Scan(&at)
	}); err != nil {
		t.Fatalf("reading the stored start: %v", err)
	}
	return at.UTC()
}

// briefWhen is the time a contact brief tells the model for one activity.
func briefWhen(t *testing.T, e *integration.Env, contact, activity ids.UUID, clock func() time.Time) string {
	t.Helper()
	for _, act := range contactbrief.FromView(contactView(t, e, contact, clock)).Recent {
		if act.ID == activity.String() {
			return act.At
		}
	}
	t.Fatalf("the brief carries no row for %v", activity)
	return ""
}

// quietDays is how long the contact page says the contact has gone quiet, or
// zero when it reports no silence.
func quietDays(t *testing.T, e *integration.Env, contact ids.UUID, clock func() time.Time) int {
	t.Helper()
	view := contactView(t, e, contact, clock)
	if view.RelationshipChanges == nil {
		t.Fatalf("the contact page withheld its relationship changes: %v", view.SectionsOmitted)
	}
	for _, change := range *view.RelationshipChanges {
		if change.Kind == crmcontracts.ContactRelationshipChangeKindWentQuiet && change.Days != nil {
			return *change.Days
		}
	}
	return 0
}

func contactView(t *testing.T, e *integration.Env, contact ids.UUID, clock func() time.Time) crmcontracts.Contact360 {
	t.Helper()
	svc := contact360.NewService(e.Pool, e.Contacts, e.Deals, e.Projects,
		consent.NewStore(InstallationDB(e.Pool)),
		comms.NewStore(InstallationDB(e.Pool), clock, activities.NewStore(InstallationDB(e.Pool))),
		ai.NewFeedbackStore(InstallationDB(e.Pool)), clock)
	view, err := svc.Assemble(e.Admin(), ids.From[ids.ContactKind](contact))
	if err != nil {
		t.Fatalf("assembling the contact: %v", err)
	}
	return view
}
