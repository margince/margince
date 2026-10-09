// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// Worklist eligibility against real records and the production composition.

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/notices"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	kevents "github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// Lead tasks are read under the rep's own-row permissions.
var leadRepPerms = principal.Permissions{
	RoleKeys: []string{"rep"},
	Objects: map[string]principal.ObjectGrant{
		"lead":     {Create: true, Read: true, Update: true},
		"contact":  {Create: true, Read: true, Update: true},
		"deal":     {Create: true, Read: true, Update: true},
		"activity": {Create: true, Read: true, Update: true},
		// A read resolves the basis it reports money in; every seeded role holds it.
		"installation_settings": {Read: true},
	},
	RowScope: principal.RowScopeOwn,
}

func measureFirstResponse(t *testing.T, e *integration.Env) {
	t.Helper()
	// The floor the setting allows, so every seeded wait below is comfortably
	// past it and no case turns on minutes.
	e.WsExec(t, `INSERT INTO setting (key, value) VALUES
		('contacts.first_response_enabled', 'true'::jsonb),
		('contacts.first_response_target_minutes', to_jsonb(15::int))
	 ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value`)
}

func ownQueue(ctx context.Context, t *testing.T, e *integration.Env) crmcontracts.Worklist {
	t.Helper()
	svc := newAttentionService(e.Pool, approvals.NewService(e.DB()), time.Now)
	page, err := svc.Worklist(ctx, "mine", "", ids.Nil, 50, "")
	if err != nil {
		t.Fatalf("reading the worklist: %v", err)
	}
	return page
}

func seedLeadTask(t *testing.T, e *integration.Env) {
	t.Helper()
	store := contacts.NewStore(e.DB())
	owner := ids.From[ids.UserKind](e.Rep1)
	lead, _, err := store.CreateLead(e.Admin(), contacts.CreateLeadInput{FullName: new("Selected prospect"), Source: "manual", OwnerID: &owner})
	if err != nil {
		t.Fatal(err)
	}
	due := time.Now().Add(-time.Hour)
	_, _, err = activities.NewStore(e.DB()).LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "task", Subject: new("Call selected prospect"), DueAt: &due,
		AssigneeID: &owner, Links: []activities.ActivityLinkInput{{EntityType: "lead", EntityID: ids.UUID(lead.Id)}},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func taskTitles(page crmcontracts.Worklist) []string {
	var titles []string
	for _, row := range page.Queue {
		if row.Source == "task" && row.Title != nil {
			titles = append(titles, *row.Title)
		}
	}
	return titles
}

func TestBareLeadsDoNotCreateWorkButPlannedLeadTasksRemain(t *testing.T) {
	for _, tracked := range []bool{false, true} {
		t.Run(map[bool]string{false: "target off", true: "target on"}[tracked], func(t *testing.T) {
			e := integration.Setup(t)
			if tracked {
				measureFirstResponse(t, e)
			}
			store := contacts.NewStore(e.DB())
			owner := ids.From[ids.UserKind](e.Rep1)
			for _, source := range []string{"manual", "webform", "import"} {
				in := contacts.CreateLeadInput{FullName: new("Bare " + source), Source: source, OwnerID: &owner}
				if source == "import" {
					in.SourceSystem = new("mirror:hubspot")
					in.SourceID = new("leads:100")
				}
				created, _, err := store.CreateLead(e.Admin(), in)
				if err != nil {
					t.Fatal(err)
				}
				if created.SlaState != nil || created.SlaDeadlineAt != nil {
					t.Fatal("bare lead received a response clock")
				}
				due := time.Now().Add(-time.Hour)
				_, _, err = activities.NewStore(e.DB()).LogActivity(e.Admin(), activities.LogActivityInput{
					Kind: "task", Subject: new("Automatic SLA escalation"), DueAt: &due, AssigneeID: &owner,
					SourceSystem: new("lead_sla"), SourceID: new(created.Id.String()),
					Links: []activities.ActivityLinkInput{{EntityType: "lead", EntityID: ids.UUID(created.Id)}},
				})
				if err != nil {
					t.Fatal(err)
				}
				_, err = notices.NewStore(e.DB()).Create(e.Admin(), notices.NewNotice{
					Recipient: owner, Kind: "lead_sla", Subject: "Automatic SLA escalation", Body: "No inquiry exists.",
					Target: notices.Target{Type: "lead", ID: ids.UUID(created.Id)},
				})
				if err != nil {
					t.Fatal(err)
				}
			}
			breaches, err := store.ScanLeadSLA(e.Admin(), time.Now().Add(48*time.Hour))
			if err != nil || len(breaches) != 0 {
				t.Fatalf("bare-lead SLA scan: %v, %v", breaches, err)
			}
			seedLeadTask(t, e)
			reader := e.As(e.Rep1, []ids.UUID{e.Team1}, leadRepPerms)
			page := ownQueue(reader, t, e)
			for _, row := range page.Queue {
				if row.Source == "lead_response" || (row.Title != nil && *row.Title == "Automatic SLA escalation") {
					t.Fatalf("bare lead reached worklist: %+v", row)
				}
			}
			if got := taskTitles(page); len(got) != 1 || got[0] != "Call selected prospect" {
				t.Fatalf("planned lead tasks = %v", got)
			}
			other := ownQueue(e.As(e.Rep2, []ids.UUID{e.Team1}, leadRepPerms), t, e)
			if len(taskTitles(other)) != 0 {
				t.Fatal("another rep received the planned task")
			}
		})
	}
}

func TestImportedLeadCreationDoesNotTriggerIntakeWorkflows(t *testing.T) {
	e := integration.Setup(t)
	e.WsExec(t, `INSERT INTO automation (key, name, trigger, action, params, enabled)
 VALUES ('route_lead', 'Follow up', '{"event_type":"lead.created"}', '{"kind":"create_task"}', '{}', true),
 ('assign_lead_owner', 'Assign intake', '{"event_type":"lead.created"}', '{"kind":"assign_owner"}', $1, true)`,
		`{"owners":["`+e.Rep1.String()+`"]}`)
	store := contacts.NewStore(e.DB())
	lead, _, err := store.CreateLead(e.Admin(), contacts.CreateLeadInput{
		FullName: new("Imported prospect"), Source: "import",
		SourceSystem: new("mirror:hubspot"), SourceID: new("leads:200"),
	})
	if err != nil {
		t.Fatal(err)
	}
	envelope := leadCreatedEnvelope(t, ids.UUID(lead.Id))
	if err := NewWorkflowEngine(e.DB()).HandleEvent(t.Context(), envelope); err != nil {
		t.Fatal(err)
	}
	if count := e.WsCount(t, `SELECT count(*) FROM activity_link WHERE lead_id = $1`, lead.Id); count != 0 {
		t.Fatalf("import created %d activities", count)
	}
	updated, err := store.GetLead(e.Admin(), ids.From[ids.LeadKind](ids.UUID(lead.Id)), storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if updated.OwnerId != nil {
		t.Fatal("import was routed as a live intake request")
	}
}

func leadCreatedEnvelope(t *testing.T, lead ids.UUID) kevents.Envelope {
	t.Helper()
	var raw []byte
	if err := integration.OwnerConn(t).QueryRow(t.Context(),
		`SELECT envelope FROM event_outbox WHERE envelope->>'type' = 'lead.created' AND envelope->'entity'->>'id' = $1`, lead.String()).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var envelope kevents.Envelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatal(err)
	}
	return envelope
}

func TestLegacyImportFollowupsStayOutButPlannedTasksAndEditsRemain(t *testing.T) {
	e := integration.Setup(t)
	e.WsExec(t, `INSERT INTO automation (key, name, trigger, action, params, enabled)
 VALUES ('route_lead', 'Follow up', '{"event_type":"lead.created"}', '{"kind":"create_task"}', '{}', true)`)
	owner := ids.From[ids.UserKind](e.Rep1)
	lead, _, err := contacts.NewStore(e.DB()).CreateLead(e.Admin(), contacts.CreateLeadInput{
		FullName: new("Imported prospect"), OwnerID: &owner, Source: "import",
		SourceSystem: new("mirror:hubspot"), SourceID: new("leads:legacy"),
	})
	if err != nil {
		t.Fatal(err)
	}
	envelope := leadCreatedEnvelope(t, ids.UUID(lead.Id))
	// The historical writer emitted an empty payload, even for an import.
	envelope.Payload = json.RawMessage(`{}`)
	if err := NewWorkflowEngine(e.DB()).HandleEvent(t.Context(), envelope); err != nil {
		t.Fatal(err)
	}
	var automatic ids.UUID
	if err := integration.OwnerConn(t).QueryRow(t.Context(),
		`SELECT activity_id FROM activity_link WHERE lead_id = $1`, lead.Id).Scan(&automatic); err != nil {
		t.Fatal(err)
	}
	store := activities.NewStore(e.DB())
	due := time.Now().Add(-time.Hour)
	planned, _, err := store.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "task", Subject: new("Follow up with the new lead"), DueAt: &due, AssigneeID: &owner,
		Links: []activities.ActivityLinkInput{{EntityType: "lead", EntityID: ids.UUID(lead.Id)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	reader := e.As(e.Rep1, []ids.UUID{e.Team1}, leadRepPerms)
	page := ownQueue(reader, t, e)
	if len(page.Queue) != 1 || page.Queue[0].Id != planned.Id.String() {
		t.Fatalf("worklist does not retain only the planned task: %+v", page.Queue)
	}
	if count := e.WsCount(t, `SELECT count(*) FROM activity_link WHERE lead_id = $1`, lead.Id); count != 2 {
		t.Fatal("legacy tasks were removed rather than filtered")
	}
	_, err = store.UpdateActivity(e.Admin(), ids.From[ids.ActivityKind](automatic), activities.UpdateActivityInput{
		Subject: new("Plan confirmed"), DueAt: &due,
	})
	if err != nil {
		t.Fatal(err)
	}
	page = ownQueue(reader, t, e)
	if len(taskTitles(page)) != 2 {
		t.Fatalf("edited task was not retained: %v", taskTitles(page))
	}
}
