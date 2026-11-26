// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/compose/integration/qualifytest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

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
	held, canceled := meetingWithLead(t, e, met, "booked"), meetingWithLead(t, e, missed, "booked")

	d.tool(t, "update_record", map[string]any{"why": "A test asks for this change.", "record_type": "activity", "id": held, "fields": map[string]any{"meeting_status": "held"}})
	d.tool(t, "update_record", map[string]any{"why": "A test asks for this change.", "record_type": "activity", "id": canceled, "fields": map[string]any{"meeting_status": "canceled"}})

	owner := sessionSeat(t, e)
	qualifytest.AssertOwnerAsked(owner, t, e.DB(), ids.MustParse(met), ids.MustParse(held))
	if tasks := qualifytest.OpenTasks(owner, t, e.DB(), ids.MustParse(missed)); len(tasks) != 0 {
		t.Errorf("a canceled meeting asked to qualify its lead: %+v", tasks)
	}
}
