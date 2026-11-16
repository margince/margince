// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// bookedWithLead logs a meeting with the lead that took place an hour ago and
// has no outcome yet.
func bookedWithLead(t *testing.T, e *apptest.AppEnv, lead string) string {
	t.Helper()
	return createdID(t, e, "/v1/activities", AnyMap{
		"kind": "meeting", "subject": "Lead demo", "meeting_status": "booked", "source": "manual",
		"occurred_at": clocktest.Now(t).Add(-time.Hour).UTC().Format(time.RFC3339),
		"links":       []AnyMap{{"entity_type": "lead", "entity_id": lead}},
	})
}

// sessionSeat is the signed-in admin as a store reader, so the Worklist and
// the lead read answer for the seat that owns the leads.
func sessionSeat(t *testing.T, e *apptest.AppEnv) context.Context {
	t.Helper()
	user := ids.MustParse(apptest.SessionUserID(t, e))
	ctx := principal.WithWorkspaceID(context.Background(), apptest.InstallationWorkspaceUUID(context.Background(), t, e.Pool))
	return principal.WithActor(principal.WithCorrelationID(ctx, ids.NewV7()), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + user.String(), UserID: user,
		SeatType: principal.SeatFull, Permissions: AdminPerms,
	})
}

// An agent that records a meeting held over MCP asks the lead's owner to
// qualify the lead. A meeting the agent records as canceled asks nobody.
func TestAnAgentMarkingALeadMeetingHeldOverMCPAsksTheOwnerToQualify(t *testing.T) {
	d := newTwoDoors(t, "lead-qualify", "read", "write")
	e := d.e
	met := createdID(t, e, "/v1/leads", AnyMap{"full_name": "Lena Prospect", "email": "lena@prospect.example"})
	missed := createdID(t, e, "/v1/leads", AnyMap{"full_name": "Milo Prospect", "email": "milo@prospect.example"})
	ownLeadByItsCreator(t, e, met)
	ownLeadByItsCreator(t, e, missed)
	held, canceled := bookedWithLead(t, e, met), bookedWithLead(t, e, missed)

	d.tool(t, "update_record", map[string]any{"record_type": "activity", "id": held, "fields": map[string]any{"meeting_status": "held"}})
	d.tool(t, "update_record", map[string]any{"record_type": "activity", "id": canceled, "fields": map[string]any{"meeting_status": "canceled"}})

	owner := sessionSeat(t, e)
	AssertOwnerAskedToQualify(owner, t, e.DB(), ids.MustParse(met), ids.MustParse(held))
	if tasks := OpenQualifyTasks(owner, t, e.DB(), ids.MustParse(missed)); len(tasks) != 0 {
		t.Errorf("a canceled meeting asked to qualify its lead: %+v", tasks)
	}
}
