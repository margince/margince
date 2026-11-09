// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// An activity field narrowed to some kinds, at every door that writes one. The
// contact timeline carries the values, and list_records filters by one.

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/contact360"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/comms"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const notValidForKind = "field_not_valid_for_kind"

// visitReportDoors is two doors on a workspace with one text field on
// activities, for every kind. It answers the doors, the field's id and its column.
func visitReportDoors(t *testing.T) (d *twoDoors, fieldID, column string) {
	t.Helper()
	d = newTwoDoorsWith(t, "activity-field-kinds",
		[]compose.Option{compose.WithSchemaPool(SchemaPool(t))}, "read", "write")
	var field struct {
		ID            string   `json:"id"`
		ColumnName    string   `json:"column_name"`
		ActivityKinds []string `json:"activity_kinds"`
	}
	if status := d.e.Call(t, "POST", "/v1/custom-fields", AnyMap{
		"object": "activity", "label": "Visit report", "type": "text", "source": "manual",
	}, nil, &field); status != http.StatusCreated {
		t.Fatalf("create the activity field → %d", status)
	}
	if field.ActivityKinds != nil {
		t.Fatalf("a field created without kinds answers %v, want null for every kind", field.ActivityKinds)
	}
	return d, field.ID, field.ColumnName
}

// narrowToMeetings sets the field's kinds through the settings PATCH.
func narrowToMeetings(t *testing.T, d *twoDoors, fieldID string) {
	t.Helper()
	var field struct {
		ActivityKinds []string `json:"activity_kinds"`
	}
	if status := d.e.Call(t, "PATCH", "/v1/custom-fields/"+fieldID, AnyMap{"activity_kinds": []string{"meeting"}}, nil, &field); status != http.StatusOK {
		t.Fatalf("narrow the field to meetings → %d", status)
	}
	if len(field.ActivityKinds) != 1 || field.ActivityKinds[0] != "meeting" {
		t.Fatalf("the narrowed field answers %v, want [meeting]", field.ActivityKinds)
	}
}

func refusedForKind(t *testing.T, what string, body []byte) {
	t.Helper()
	if !strings.Contains(string(body), notValidForKind) {
		t.Errorf("%s was refused without %s: %s", what, notValidForKind, body)
	}
}

func TestAMeetingOnlyFieldIsRefusedOnACallAtEveryDoor(t *testing.T) {
	d, fieldID, column := visitReportDoors(t)
	held := decodeActivity(t, d.restStatus(t, "POST", "/v1/activities", AnyMap{
		"kind": "call", "subject": "Call before the narrowing", "source": "manual", column: "logged while it applied",
	}, http.StatusCreated))
	heldID, _ := held["id"].(string)
	narrowToMeetings(t, d, fieldID)

	refusedForKind(t, "a call logged over REST", d.restStatus(t, "POST", "/v1/activities", AnyMap{
		"kind": "call", "subject": "Quick call", "source": "manual", column: "x",
	}, http.StatusUnprocessableEntity))
	if refused := d.toolRefused(t, "log_activity", map[string]any{"kind": "call", "subject": "Quick call", column: "x"}); !strings.Contains(refused, column) {
		t.Errorf("log_activity refused a call's value without naming %s: %s", column, refused)
	}
	meeting := decodeActivity(t, d.restStatus(t, "POST", "/v1/activities", AnyMap{
		"kind": "meeting", "subject": "Plant walk-through", "source": "manual", column: "saw the new line",
	}, http.StatusCreated))
	if meeting[column] != "saw the new line" {
		t.Errorf("a meeting carries %v under %s, want its value", meeting[column], column)
	}

	if got := decodeActivity(t, d.restStatus(t, "GET", "/v1/activities/"+heldID, nil, http.StatusOK)); got[column] != "logged while it applied" {
		t.Errorf("a value stored before the narrowing reads %v, want it kept", got[column])
	}
	refusedForKind(t, "a call patched over REST", d.restStatus(t, "PATCH", "/v1/activities/"+heldID, AnyMap{column: "changed"}, http.StatusUnprocessableEntity))
	if refused := d.toolRefused(t, "update_record", map[string]any{
		"record_type": "activity", "id": heldID, "fields": map[string]any{column: "changed"},
	}); !strings.Contains(refused, column) {
		t.Errorf("update_record refused a call's value without naming %s: %s", column, refused)
	}
	cleared := decodeActivity(t, d.restStatus(t, "PATCH", "/v1/activities/"+heldID, AnyMap{column: nil}, http.StatusOK))
	if _, still := cleared[column]; still {
		t.Errorf("a null patch on the call left %v under %s", cleared[column], column)
	}
}

func TestTheFieldSettingsRefuseKindsTheFieldCannotTake(t *testing.T) {
	d, fieldID, _ := visitReportDoors(t)
	if status := d.e.Call(t, "PATCH", "/v1/custom-fields/"+fieldID, AnyMap{"activity_kinds": []string{"visit"}}, nil, nil); status != http.StatusUnprocessableEntity {
		t.Errorf("a kind outside the activity kinds → %d, want 422", status)
	}
	var deal struct {
		ID string `json:"id"`
	}
	if status := d.e.Call(t, "POST", "/v1/custom-fields", AnyMap{
		"object": "deal", "label": "Region", "type": "text", "source": "manual", "activity_kinds": []string{"meeting"},
	}, nil, &deal); status != http.StatusUnprocessableEntity {
		t.Errorf("a deal field created with activity kinds → %d, want 422", status)
	}
	narrowToMeetings(t, d, fieldID)
	var every struct {
		ActivityKinds []string `json:"activity_kinds"`
	}
	if status := d.e.Call(t, "PATCH", "/v1/custom-fields/"+fieldID, AnyMap{"activity_kinds": []string{}}, nil, &every); status != http.StatusOK || every.ActivityKinds != nil {
		t.Errorf("an empty kind list → %d with %v, want 200 and null for every kind", status, every.ActivityKinds)
	}
}

func TestListRecordsNarrowsActivitiesByACustomField(t *testing.T) {
	d, _, column := visitReportDoors(t)
	wanted := decodeActivity(t, d.restStatus(t, "POST", "/v1/activities", AnyMap{
		"kind": "meeting", "subject": "Plant walk-through", "source": "manual", column: "north gate",
	}, http.StatusCreated))
	d.restStatus(t, "POST", "/v1/activities", AnyMap{
		"kind": "meeting", "subject": "Second walk-through", "source": "manual", column: "south gate",
	}, http.StatusCreated)

	var listed struct {
		Records []struct {
			ID string `json:"id"`
		} `json:"records"`
	}
	raw := d.tool(t, "list_records", map[string]any{"record_type": "activity", "filters": map[string]any{column: "north gate"}})
	if err := json.Unmarshal(raw, &listed); err != nil {
		t.Fatalf("decode list_records %s: %v", raw, err)
	}
	if len(listed.Records) != 1 || listed.Records[0].ID != wanted["id"] {
		t.Errorf("list_records by %s listed %+v, want the north-gate meeting alone", column, listed.Records)
	}
	if refused := d.toolRefused(t, "list_records", map[string]any{"record_type": "deal", "filters": map[string]any{column: "x"}}); !strings.Contains(refused, column) {
		t.Errorf("a deal list refused a cf_ filter without naming it: %s", refused)
	}
	if refused := d.toolRefused(t, "list_records", map[string]any{"record_type": "activity", "filters": map[string]any{"cf_nobody_defined": "x"}}); !strings.Contains(refused, "cf_nobody_defined") {
		t.Errorf("an unknown cf_ key was not refused as a field fault naming it: %s", refused)
	}
}

func TestTheContactTimelineCarriesActivityCustomFieldValues(t *testing.T) {
	e := Setup(t)
	store, column := visitNoteStore(t, e)
	author := e.As(e.Rep1, []ids.UUID{e.Team1}, activityLifecyclePerms)
	colleague := e.As(e.Rep3, []ids.UUID{e.Team2}, activityLifecyclePerms)
	contact := e.SeedContact(t, "Dana Buyer", &e.Rep1)
	links := []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}}
	open, limited := "Plant walk-through", "Pricing talk"
	if _, _, err := store.LogActivity(author, activities.LogActivityInput{
		Kind: activities.KindMeeting, Subject: &open, Source: "manual", Links: links,
		CustomFields: map[string]any{column: "second shift starts at six"},
	}); err != nil {
		t.Fatalf("log: %v", err)
	}
	private, _, err := store.LogActivity(author, activities.LogActivityInput{
		Kind: activities.KindMeeting, Subject: &limited, Source: "manual", Links: links,
		CustomFields: map[string]any{column: "asked for a discount"},
	})
	if err != nil {
		t.Fatalf("log: %v", err)
	}
	if _, err := store.SetAudience(author, ids.From[ids.ActivityKind](ids.UUID(private.Id)),
		activities.SetAudienceInput{Audience: "participants"}); err != nil {
		t.Fatalf("limiting: %v", err)
	}

	page, err := contact360.NewService(e.Pool, e.Contacts, e.Deals, e.Projects, consent.NewStore(e.DB()),
		comms.NewStore(e.DB(), func() time.Time { return clocktest.Now(t) }, activities.NewStore(e.DB())), ai.NewFeedbackStore(e.DB()),
		func() time.Time { return clocktest.Now(t) }).WithActivityFields(store).
		Assemble(colleague, ids.From[ids.ContactKind](contact))
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	if page.Activities == nil || len(page.Activities.Data) != 2 {
		t.Fatalf("the contact timeline holds %v, want both meetings", page.Activities)
	}
	withheldRows := 0
	for _, row := range page.Activities.Data {
		withheld := row.ContentState != nil && *row.ContentState == crmcontracts.ActivityContentStateWithheld
		if withheld {
			withheldRows++
		}
		got := row.AdditionalProperties[column]
		if withheld && got != nil {
			t.Errorf("the withheld meeting carries %v under %s, want nothing", got, column)
		}
		if !withheld && got != "second shift starts at six" {
			t.Errorf("the open meeting carries %v under %s, want the logged value", got, column)
		}
	}
	if withheldRows != 1 {
		t.Errorf("the colleague's timeline withholds %d meetings, want the limited one", withheldRows)
	}
}
