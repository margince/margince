// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// Each filter the seam offers lands on the field the HTTP door fills for the
// same query parameter. The expected values are written out, so a filter bound
// to a sibling field fails here.
func TestActivityFiltersLandWhereTheRouteFillsThem(t *testing.T) {
	entity, assignee, project := ids.NewV7(), ids.NewV7(), ids.NewV7()
	var in ListActivitiesInput
	err := activityListFilters.Apply(&in, map[string]string{
		"kind": "email", "channel_provider": "gmail", "entity_type": "contact", "entity_id": entity.String(),
		"assignee_id": assignee.String(), "project_id": project.String(), "thread_key": "thread-7",
		"occurred_after": "2026-10-01T00:00:00Z", "occurred_before": "2026-10-15T09:30:00+02:00",
		"waiting_reply": "true", "include_archived": "true", "sort": "-occurred_at",
	})
	if err != nil {
		t.Fatalf("applying every filter: %v", err)
	}
	words := map[string]*string{
		"kind": in.Kind, "channel_provider": in.ChannelProvider, "entity_type": in.EntityType,
		"thread_key": in.ThreadKey, "sort": in.Sort,
	}
	for name, want := range map[string]string{
		"kind": "email", "channel_provider": "gmail", "entity_type": "contact", "thread_key": "thread-7", "sort": "-occurred_at",
	} {
		if got := words[name]; got == nil || *got != want {
			t.Errorf("%s landed as %v, want %q", name, got, want)
		}
	}
	if in.EntityID == nil || *in.EntityID != entity {
		t.Errorf("entity_id landed as %v, want %s", in.EntityID, entity)
	}
	if in.AssigneeID == nil || in.AssigneeID.UUID != assignee {
		t.Errorf("assignee_id landed as %v, want %s", in.AssigneeID, assignee)
	}
	if in.WithinProjectID == nil || in.WithinProjectID.UUID != project {
		t.Errorf("project_id landed as %v, want %s", in.WithinProjectID, project)
	}
	after := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	before := time.Date(2026, 10, 15, 7, 30, 0, 0, time.UTC)
	if in.OccurredAfter == nil || !in.OccurredAfter.Equal(after) {
		t.Errorf("occurred_after landed as %v, want %s", in.OccurredAfter, after)
	}
	if in.OccurredBefore == nil || !in.OccurredBefore.Equal(before) {
		t.Errorf("occurred_before landed as %v, want %s", in.OccurredBefore, before)
	}
	if !in.WaitingReply || !in.IncludeArchived {
		t.Errorf("waiting_reply=%v include_archived=%v, want both on", in.WaitingReply, in.IncludeArchived)
	}
	if in.WaitingReplyAsOf != nil {
		t.Error("waiting_reply fixed an instant itself; the store must judge it on its own clock")
	}
}

// A malformed operand is refused under the filter's own name, saying the form
// it takes, and leaves the input as it was.
func TestAnActivityFilterOperandOfTheWrongShapeIsRefused(t *testing.T) {
	for _, c := range []struct{ filter, operand, says string }{
		{"entity_id", "contact-42", "uuid"},
		{"occurred_after", "yesterday", "RFC 3339"},
		{"occurred_before", "2026-10-15", "RFC 3339"},
		{"waiting_reply", "yes", "true"},
	} {
		var in ListActivitiesInput
		err := activityListFilters.Apply(&in, map[string]string{c.filter: c.operand})
		var refused *storekit.FilterError
		if !errors.As(err, &refused) || refused.Name != c.filter {
			t.Errorf("%s=%q answered %v, want a refusal naming %s", c.filter, c.operand, err, c.filter)
			continue
		}
		if _, _, message := refused.FieldFault(); !strings.Contains(message, c.says) {
			t.Errorf("%s's refusal %q does not say it takes %s", c.filter, message, c.says)
		}
		if in.EntityID != nil || in.OccurredAfter != nil || in.OccurredBefore != nil || in.WaitingReply {
			t.Errorf("%s=%q was refused yet narrowed the input: %+v", c.filter, c.operand, in)
		}
	}
}

// The timeline offers its filters on the activity type, and nothing on a type
// this module does not list.
func TestOnlyTheActivityTypeOffersTheTimelineFilters(t *testing.T) {
	p := &Provider{}
	want := []string{
		"assignee_id", "channel_provider", "entity_id", "entity_type", "include_archived", "kind",
		"occurred_after", "occurred_before", "project_id", "reminder_due", "sort", "source_activity_id", "thread_key", "waiting_reply",
	}
	if got := p.ListFilters(datasource.EntityActivity); !slices.Equal(got, want) {
		t.Errorf("activity offers %v, want %v", got, want)
	}
	if got := p.ListFilters(datasource.EntityContact); got != nil {
		t.Errorf("contact is not this module's to list, yet it offers %v", got)
	}
}

// The seam refuses what the route refuses, before any row is read. That is a
// type this module does not page, an unknown filter, and half the entity pair.
func TestTheTimelineSeamRefusesBeforeItReads(t *testing.T) {
	p := &Provider{}
	ctx := context.Background()

	_, _, _, err := p.SearchEntity(ctx, datasource.EntityDeal, nil, 10, nil, nil)
	var unsupported *datasource.UnsupportedEntityError
	if !errors.As(err, &unsupported) || unsupported.Type != string(datasource.EntityDeal) {
		t.Errorf("paging deals through the timeline answered %v, want an unsupported-type refusal", err)
	}

	_, _, _, err = p.SearchEntity(ctx, datasource.EntityActivity, nil, 10, nil, map[string]string{"owner_id": ids.NewV7().String()})
	var unknown *storekit.FilterError
	if !errors.As(err, &unknown) || unknown.Name != "owner_id" {
		t.Errorf("an unknown filter answered %v, want a refusal naming owner_id", err)
	}

	for _, half := range []map[string]string{
		{"entity_type": "contact"},
		{"entity_id": ids.NewV7().String()},
	} {
		_, _, _, err = p.SearchEntity(ctx, datasource.EntityActivity, nil, 10, nil, half)
		var pair *httperr.DetailedError
		if !errors.As(err, &pair) || pair.Status != http.StatusUnprocessableEntity ||
			len(pair.Fields) == 0 || pair.Fields[0].Code != "requires_pair" {
			t.Errorf("filtering by %v alone answered %v, want the requires_pair refusal", half, err)
		}
	}
}
