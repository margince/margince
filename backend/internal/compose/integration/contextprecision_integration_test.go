// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/search"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
	"github.com/margince/margince/backend/internal/shared/ports/retrieval"
)

// itemFor is the assembled item naming one record in one section.
func itemFor(t *testing.T, assembled retrieval.Context, section string, id ids.UUID) retrieval.Item {
	t.Helper()
	for _, sec := range assembled.Sections {
		for _, item := range sec.Items {
			if sec.Name == section && item.Ref.ID == id {
				return item
			}
		}
	}
	t.Fatalf("no %s item names %s in %+v", section, id, assembled.Sections)
	return retrieval.Item{}
}

// A day entry reaches an agent's context as a day, named by its own date. In
// Auckland its stored noon is the next morning, and the date still holds.
func TestAnAssembledDayEntryIsADay(t *testing.T) {
	e := SetupSearch(t)
	f := seedMeetingFixture(t, e)
	if _, err := e.Owner.Exec(context.Background(),
		`UPDATE setting SET value = '"Pacific/Auckland"'::jsonb WHERE key = 'installation.timezone'`); err != nil {
		t.Fatal(err)
	}
	subject, date := "Trade fair", time.Date(2026, 3, 9, 0, 0, 0, 0, time.UTC)
	writer := activities.NewStore(database.BindTo(e.Pool, ids.From[ids.WorkspaceKind](e.WS)))
	logged, _, err := writer.LogActivity(e.AsFullUser(), activities.LogActivityInput{
		Kind: activities.KindMeeting, Subject: &subject, OccurredAt: &date, OccurredPrecision: activities.PrecisionDay,
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: f.organizer}}, Source: "manual",
	})
	if err != nil {
		t.Fatalf("logging the day meeting: %v", err)
	}
	meeting := ids.UUID(logged.Id)

	prep, err := prepFor(e.Admin(), t, e, meeting)
	if err != nil {
		t.Fatalf("preparing for the meeting: %v", err)
	}
	profile := itemFor(t, prep, "profile", meeting)
	if !profile.DateOnly || !strings.HasSuffix(profile.Summary, "on 2026-03-09") {
		t.Errorf("the profile reads %q (date only: %v), want the date alone", profile.Summary, profile.DateOnly)
	}

	around, err := search.NewRetriever(e.Store, nil).AssembleContext(e.Admin(),
		datasource.EntityRef{Type: datasource.EntityContact, ID: f.organizer}, retrieval.AssembleOptions{MaxItems: 5})
	if err != nil {
		t.Fatalf("assembling the contact: %v", err)
	}
	if touch := itemFor(t, around, "recent_touches", meeting); !touch.DateOnly {
		t.Errorf("the contact's recent touch %q is not marked as a day", touch.Summary)
	}
}
