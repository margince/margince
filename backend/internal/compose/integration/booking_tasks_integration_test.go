// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
	"github.com/margince/margince/backend/internal/shared/ports/workflow"
)

const bookerAddress = "lead-a@booker.example"

type bookedLead struct {
	e    *apptest.AppEnv
	host ids.UUID
	lead ids.UUID
	task ids.UUID
}

func setupBookedLead(t *testing.T) *bookedLead {
	t.Helper()
	e := setupBookingApp(t)
	e.BootstrapWorkspace(t)
	apptest.EnableGoogleBookingPage(t, e)
	b := &bookedLead{e: e}
	if err := e.Owner.QueryRow(context.Background(), `SELECT host_user_id FROM booking_page`).Scan(&b.host); err != nil {
		t.Fatal(err)
	}
	b.lead = SeedIDRow(t, e.Owner, `INSERT INTO lead (id, full_name, email, status, source, captured_by, owner_id) VALUES ($1,'Lead A',$2,'new','manual','human:x',$3)`, bookerAddress, b.host)
	var task struct {
		ID ids.UUID `json:"id"`
	}
	status := e.Call(t, "POST", "/v1/activities", AnyMap{
		"kind": "task", "subject": "Send Lead A the booking link again", "source": "manual", "assignee_id": b.host,
		"links": []AnyMap{{"entity_type": "lead", "entity_id": b.lead}},
	}, nil, &task)
	if status != http.StatusCreated {
		t.Fatalf("creating the host's task → %d", status)
	}
	b.task = task.ID
	return b
}

func (b *bookedLead) book(t *testing.T) {
	t.Helper()
	monday := nextMonday()
	body := AnyMap{"start": monday.Add(time.Hour), "end": monday.Add(90 * time.Minute), "booker": AnyMap{"name": "Lead A", "email": bookerAddress}, "consent": AnyMap{"policy_version": "pp-2026-01", "wording": "Contact me about this meeting."}}
	if status := publicCall(t, b.e, "POST", "/v1/public/booking/"+bookingSlug(t, b.e), body, nil, nil); status != http.StatusCreated && status != http.StatusOK {
		t.Fatalf("public booking → %d", status)
	}
}

func (b *bookedLead) taskDone(t *testing.T) bool {
	t.Helper()
	var done bool
	if err := b.e.Owner.QueryRow(context.Background(), `SELECT is_done FROM activity WHERE id=$1`, b.task).Scan(&done); err != nil {
		t.Fatal(err)
	}
	return done
}

func TestABookingOnTheHostsLeadThroughThePublicPageClosesTheHostsTaskAndCancelReopensItOnce(t *testing.T) {
	b := setupBookedLead(t)
	b.book(t)
	if !b.taskDone(t) {
		t.Fatal("the host's own task stayed open after the lead booked")
	}
	var meeting ids.UUID
	if err := b.e.Owner.QueryRow(context.Background(), `SELECT id FROM activity WHERE kind='meeting'`).Scan(&meeting); err != nil {
		t.Fatal(err)
	}
	if _, err := b.e.Owner.Exec(context.Background(), `UPDATE activity SET meeting_status='canceled' WHERE id=$1`, meeting); err != nil {
		t.Fatal(err)
	}
	handler := activities.QuietReminderReopenWorkflows(activities.NewStore(b.e.DB()))[0]
	wsID := apptest.InstallationWorkspaceUUID(context.Background(), t, b.e.Pool)
	ctx := principal.SystemActing(context.Background(), "system:test")
	canceled := crmcontracts.MeetingWasCanceled
	payload, err := json.Marshal(crmcontracts.PublicEventActivityUpdated{ChangedFields: crmcontracts.PublicEventActivityChangedFields{MeetingStatus: &canceled}})
	if err != nil {
		t.Fatal(err)
	}
	event := workflow.Event{ID: ids.NewV7(), Type: "activity.updated", WorkspaceID: wsID, Entity: datasource.EntityRef{ID: meeting}, Payload: payload}
	for run, wantReopened := range []int{1, 0} {
		matched, err := handler.Match(ctx, event)
		if err != nil || !matched {
			t.Fatalf("run %d: match=%v %v", run, matched, err)
		}
		effect, err := handler.Plan(ctx, event)
		if err != nil {
			t.Fatal(err)
		}
		result, err := handler.Apply(ctx, event, effect, nil)
		if err != nil || (len(result.Applied) > 0) != (wantReopened == 1) {
			t.Fatalf("run %d: applied=%d %v, want reopened=%d", run, len(result.Applied), err, wantReopened)
		}
		if b.taskDone(t) {
			t.Fatalf("run %d: the called-off booking left the task closed", run)
		}
	}
	var reopens int
	if err := b.e.Owner.QueryRow(context.Background(), `SELECT count(*) FROM audit_log WHERE entity_id=$1 AND evidence ? 'task_reopened_by'`, b.task).Scan(&reopens); err != nil || reopens != 1 {
		t.Fatalf("reopen audit rows=%d, %v; want exactly one", reopens, err)
	}
}

func TestABookingOnALeadTheHostMayNoLongerWriteClosesNothing(t *testing.T) {
	b := setupBookedLead(t)
	colleague := SeedIDRow(t, b.e.Owner, `INSERT INTO app_user (id, email, display_name) VALUES ($1, 'colleague@booker.test', 'Colleague')`)
	if _, err := b.e.Owner.Exec(context.Background(), `UPDATE lead SET owner_id=$2 WHERE id=$1`, b.lead, colleague); err != nil {
		t.Fatal(err)
	}
	if _, err := b.e.Owner.Exec(context.Background(), `UPDATE role_assignment SET role_id=(SELECT id FROM role WHERE key='rep' AND archived_at IS NULL) WHERE user_id=$1`, b.host); err != nil {
		t.Fatal(err)
	}
	b.book(t)
	if b.taskDone(t) {
		t.Fatal("an anonymous booking closed the host's task on a lead the host may no longer write")
	}
	var meetings int
	if err := b.e.Owner.QueryRow(context.Background(), `SELECT count(*) FROM activity WHERE kind='meeting'`).Scan(&meetings); err != nil || meetings != 1 {
		t.Fatalf("the booking itself must still land: meetings=%d, %v", meetings, err)
	}
}
