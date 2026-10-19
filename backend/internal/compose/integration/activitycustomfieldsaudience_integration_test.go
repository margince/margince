// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// An activity's custom-field values are content. A colleague outside a limited
// activity's audience reads the row without them, and cannot find it by them.

import (
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	customfieldsmod "github.com/margince/margince/backend/internal/modules/customfields"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestALimitedActivityWithholdsItsCustomFieldValues(t *testing.T) {
	e := Setup(t)
	catalog := customfieldsmod.NewService(e.Pool, SchemaPool(t))
	field, err := catalog.Create(e.As(e.Rep1, nil, CustomFieldAdminPerms), customfieldsmod.FieldSpec{
		Object: "activity", Label: "Visit note", Type: customfieldsmod.TypeText, Source: "manual",
	})
	if err != nil {
		t.Fatalf("create the activity field: %v", err)
	}
	column := *field.ColumnName
	store := activities.NewStore(e.DB()).WithFieldCatalog(catalog)
	author := e.As(e.Rep1, []ids.UUID{e.Team1}, activityLifecyclePerms)
	colleague := e.As(e.Rep3, []ids.UUID{e.Team2}, activityLifecyclePerms)
	contact := e.SeedContact(t, "Dana Buyer", &e.Rep1)

	subject := "Plant walk-through"
	logged, _, err := store.LogActivity(author, activities.LogActivityInput{
		Kind: activities.KindMeeting, Subject: &subject, Source: "manual",
		Links:        []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}},
		CustomFields: map[string]any{column: "asked about the night shift"},
	})
	if err != nil {
		t.Fatalf("log: %v", err)
	}
	id := ids.From[ids.ActivityKind](ids.UUID(logged.Id))
	if _, err := store.SetAudience(author, id, activities.SetAudienceInput{Audience: "participants"}); err != nil {
		t.Fatalf("limiting: %v", err)
	}

	mine, err := store.GetActivity(author, id, storekit.LiveOnly)
	if err != nil || mine.AdditionalProperties[column] != "asked about the night shift" {
		t.Fatalf("the author's own read = %v (%v), want the value", mine.AdditionalProperties, err)
	}
	theirs, err := store.GetActivity(colleague, id, storekit.LiveOnly)
	if err != nil {
		t.Fatalf("the colleague's read: %v, want the withheld row", err)
	}
	if *theirs.ContentState != crmcontracts.ActivityContentStateWithheld || theirs.AdditionalProperties[column] != nil {
		t.Errorf("the colleague's read = %+v, want content_state=withheld with no %s", theirs, column)
	}

	byValue := activities.ListActivitiesInput{CustomFilters: map[string]string{column: "asked about the night shift"}}
	if found, _, err := store.ListActivities(author, byValue); err != nil || len(found) != 1 {
		t.Fatalf("the author filtering on the value found %d rows (%v), want the meeting", len(found), err)
	}
	if found, _, err := store.ListActivities(colleague, byValue); err != nil || len(found) != 0 {
		t.Errorf("the colleague filtering on a value they may not read found %d rows (%v), want none", len(found), err)
	}
}
