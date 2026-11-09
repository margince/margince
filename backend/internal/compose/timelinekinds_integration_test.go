// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A record timeline narrowed to several kinds pages over those kinds alone, so
// newer work of another kind cannot fill the page and hide them.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// seedBusyContact gives a contact 30 recent notes and, two months back, one
// email each way: the mail sits behind a full page of notes.
func seedBusyContact(t *testing.T, e *integration.Env, author context.Context) ids.UUID {
	t.Helper()
	contact := e.SeedContact(t, "Dana Buyer", &e.Rep1)
	base := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	logOne := func(in activities.LogActivityInput) {
		in.Links = []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}}
		if _, _, err := e.Activities.LogActivity(author, in); err != nil {
			t.Fatalf("logging a %s: %v", in.Kind, err)
		}
	}
	for day := range 30 {
		at := base.AddDate(0, 0, -day)
		logOne(activities.LogActivityInput{Kind: "note", Body: new("Spoke about the rollout"), OccurredAt: &at})
	}
	for _, mail := range []struct {
		direction string
		daysAgo   int
	}{{"inbound", 60}, {"outbound", 59}} {
		at := base.AddDate(0, 0, -mail.daysAgo)
		logOne(activities.LogActivityInput{
			Kind: "email", Subject: new("Pricing question"), Body: new("What does it cost?"),
			Direction: new(mail.direction), OccurredAt: &at,
		})
	}
	return contact
}

func listTimeline(t *testing.T, ctx context.Context, e *integration.Env, params crmcontracts.ListActivitiesParams) crmcontracts.ActivityListResponse {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/v1/activities", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	newActivitiesHandlers(e.Pool).ListActivities(rec, req, params)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /activities answered %d: %s", rec.Code, rec.Body.String())
	}
	var page crmcontracts.ActivityListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatalf("decoding the page: %v", err)
	}
	return page
}

func TestATimelineNarrowedToSeveralKindsPagesOverThoseKindsAlone(t *testing.T) {
	e := integration.Setup(t)
	reader := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AccountRepPerms)
	contact := seedBusyContact(t, e, reader)
	entity, id := crmcontracts.ListActivitiesParamsEntityTypeContact, openapi_types.UUID(contact)
	one := 1
	mail := []crmcontracts.ListActivitiesParamsKind{crmcontracts.ListActivitiesParamsKindEmail, crmcontracts.ListActivitiesParamsKindMessage}
	params := crmcontracts.ListActivitiesParams{EntityType: &entity, EntityId: &id, Kind: &mail, Limit: &one}

	var seen []crmcontracts.ActivityKind
	first := listTimeline(t, reader, e, params)
	if !first.Page.HasMore || first.Page.NextCursor == nil {
		t.Fatalf("first page of one: has_more=%v cursor=%v, want a second page holding the other email", first.Page.HasMore, first.Page.NextCursor)
	}
	params.Cursor = first.Page.NextCursor
	second := listTimeline(t, reader, e, params)
	if second.Page.HasMore {
		t.Errorf("second page says more follows; the two emails are all the mail there is")
	}
	for _, row := range append(first.Data, second.Data...) {
		seen = append(seen, row.Kind)
	}
	if len(seen) != 2 || seen[0] != crmcontracts.ActivityKindEmail || seen[1] != crmcontracts.ActivityKindEmail {
		t.Errorf("kind=email&kind=message read %v, want the two emails and no note", seen)
	}

	notes := []crmcontracts.ListActivitiesParamsKind{crmcontracts.ListActivitiesParamsKindNote}
	twenty := 20
	page := listTimeline(t, reader, e, crmcontracts.ListActivitiesParams{EntityType: &entity, EntityId: &id, Kind: &notes, Limit: &twenty})
	if len(page.Data) != 20 || !page.Page.HasMore {
		t.Errorf("kind=note read %d rows, has_more=%v; want a full page of 20 with more to come", len(page.Data), page.Page.HasMore)
	}
	for _, row := range page.Data {
		if row.Kind != crmcontracts.ActivityKindNote {
			t.Errorf("kind=note listed a %s", row.Kind)
		}
	}
}
