// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/automation"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/database"
	kevents "github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestTwoMeetingRecapRulesStageOneDraftAfterNotesAndHeld(t *testing.T) {
	e := integration.Setup(t)
	e.GrantRole(t, e.AdminUser, "admin")
	rbac, err := identity.NewService(e.Pool).EffectiveRBAC(e.Admin(), e.WS, e.AdminUser)
	if err != nil {
		t.Fatal(err)
	}
	ctx := e.As(e.AdminUser, rbac.TeamIDs, rbac.Permissions)
	db := database.BindTo(e.Pool, ids.From[ids.WorkspaceKind](e.WS))
	now := time.Date(2026, 7, 15, 14, 0, 0, 0, time.UTC)
	contact, err := e.Contacts.CreateContact(ctx, contacts.CreateContactInput{
		FullName: "Zora Buyer", Source: "manual",
		Emails: []contacts.ContactEmailInput{{Email: "buyer@customer.example", EmailType: "work", IsPrimary: true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	row, _, err := e.Activities.LogActivity(ctx, activities.LogActivityInput{
		Kind: "meeting", Source: "manual", Subject: new("Review"), Body: new(""),
		MeetingStatus: new("held"), OccurredAt: new(now.Add(-time.Hour)),
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: ids.UUID(contact.Id)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	historical, _, err := e.Activities.LogActivity(ctx, activities.LogActivityInput{
		Kind: "meeting", Source: "manual", Subject: new("Earlier review"), Body: new("The original notes."),
		MeetingStatus: new("held"), OccurredAt: new(now.Add(-2 * time.Hour)),
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: ids.UUID(contact.Id)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	activatedAt := historical.CreatedAt.Add(time.Second)
	rules := automation.NewAutomationStore(db).SetClock(func() time.Time { return activatedAt })
	for _, name := range []string{"First recap", "Second recap"} {
		rule, err := rules.Create(ctx, automation.CreateAutomationInput{Key: "post_meeting_recap", Name: name, Params: map[string]any{}})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := rules.Update(ctx, rule.ID, automation.UpdateAutomationInput{Enabled: new(true)}); err != nil {
			t.Fatal(err)
		}
	}
	engine := workflowEngineWithDrafter(db, &changingMeetingDraft{})
	dispatch := func(kind string, source crmcontracts.Id, becameReady bool) {
		t.Helper()
		var raw json.RawMessage
		err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, `SELECT envelope FROM event_outbox
    WHERE envelope->>'type'=@kind AND envelope->'entity'->>'id'=@id
    ORDER BY created_at DESC, id DESC LIMIT 1`, pgx.NamedArgs{"kind": kind, "id": source.String()}).Scan(&raw)
		})
		if err != nil {
			t.Fatal(err)
		}
		var event kevents.Envelope
		if err := json.Unmarshal(raw, &event); err != nil {
			t.Fatal(err)
		}
		if kind == "activity.updated" {
			var update crmcontracts.PublicEventActivityUpdated
			if err := json.Unmarshal(event.Payload, &update); err != nil {
				t.Fatal(err)
			}
			flag := update.ChangedFields.MeetingRecapBecameReady
			if (flag != nil && *flag) != becameReady {
				t.Fatalf("ready transition=%v, want %v", flag, becameReady)
			}
		}
		if kind == "activity.updated" {
			event.OccurredAt = activatedAt.Add(time.Second)
		}
		if err := engine.HandleEvent(ctx, event); err != nil {
			t.Fatal(err)
		}
	}
	count := func(want int) {
		t.Helper()
		if got := e.WsCount(t, `SELECT count(*) FROM approval WHERE kind='held_draft'`); got != want {
			t.Fatalf("draft approvals = %d, want %d", got, want)
		}
	}
	dispatch("activity.captured", row.Id, false)
	count(0)
	historicalID := ids.From[ids.ActivityKind](ids.UUID(historical.Id))
	if _, err := e.Activities.UpdateActivity(ctx, historicalID, activities.UpdateActivityInput{Body: new("An edit to the old notes.")}); err != nil {
		t.Fatal(err)
	}
	dispatch("activity.updated", historical.Id, false)
	count(0)
	id := ids.From[ids.ActivityKind](ids.UUID(row.Id))
	if _, err := e.Activities.UpdateActivity(ctx, id, activities.UpdateActivityInput{Body: new("Agreed the next steps.")}); err != nil {
		t.Fatal(err)
	}
	dispatch("activity.updated", row.Id, true)
	count(1)
	dispatch("activity.updated", row.Id, true)
	if _, err := e.Activities.UpdateActivity(ctx, id, activities.UpdateActivityInput{Body: new("Agreed the next steps and a date.")}); err != nil {
		t.Fatal(err)
	}
	dispatch("activity.updated", row.Id, false)
	count(1)
	if got := e.WsCount(t, `SELECT count(*) FROM automation_effect_claim WHERE handler='post_meeting_recap' AND applied_at IS NOT NULL`); got != 1 {
		t.Fatalf("confirmed meeting claims = %d, want 1", got)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM event_outbox WHERE envelope->>'type'='email.sent'`); got != 0 {
		t.Fatalf("sent email events = %d, want 0", got)
	}
}

type changingMeetingDraft struct{ calls int }

func (d *changingMeetingDraft) DraftEmail(context.Context, ids.UUID, string) (string, string, error) {
	d.calls++
	return "Meeting recap", fmt.Sprintf("Draft version %d", d.calls), nil
}
