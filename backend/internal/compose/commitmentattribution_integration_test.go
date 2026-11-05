// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"encoding/json"
	"testing"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestAnOutboundReportKeepsTheCustomersPromiseOnTheCustomer(t *testing.T) {
	e, reply := seedMailedPromise(t, "active")
	var payload extractPayload
	if err := json.Unmarshal([]byte(reply), &payload); err != nil {
		t.Fatal(err)
	}
	event := &(*payload.Events)[0]
	contact := e.WsScalar(t, `SELECT contact_id::text FROM activity_participant WHERE activity_id = $1 AND role = 'to'`, event.MessageID)
	event.Quote = "You agreed to send the payment by Friday."
	event.Summary = "Send the payment"
	event.PromiserRef = "contact:" + contact
	e.WsExec(t, `UPDATE activity SET body = $2 WHERE id = $1`, event.MessageID, event.Quote)
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if err := runSignalPass(t, e, string(encoded)); err != nil {
		t.Fatal(err)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM activity WHERE kind = 'task'`); got != 0 {
		t.Fatalf("a reported customer promise created %d colleague tasks", got)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM conversation_claim WHERE kind = 'commitment_theirs' AND contact_id = $1`, contact); got != 1 {
		t.Fatalf("customer dependencies = %d, want one", got)
	}
}

func TestAnUnresolvedPromiserDoesNotBecomeTheSender(t *testing.T) {
	e, reply := seedMailedPromise(t, "active")
	reply = mailedReplyWithoutPromiser(t, reply)
	if err := runSignalPass(t, e, reply); err != nil {
		t.Fatal(err)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM activity WHERE kind = 'task'`); got != 0 {
		t.Fatalf("unresolved attribution created %d assigned tasks", got)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM approval WHERE kind = 'commitment_task'`); got != 0 {
		t.Fatalf("a promise nobody can be named for staged %d proposals, want none", got)
	}
}

func TestAMeetingReportKeepsTheCustomersPromiseOnTheCustomer(t *testing.T) {
	e := setupTranscript(t)
	customer := e.SeedContact(t, "Ines Huber", &e.Rep1)
	body, source := "Mira: Ines Huber agreed to send the payment by Friday.", "transcript"
	meeting, _, err := e.Activities.LogActivity(e.ctx, activities.LogActivityInput{
		Kind: "meeting", Body: &body, SourceSystem: &source, Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: customer}},
	})
	if err != nil {
		t.Fatal(err)
	}
	e.activity = ids.From[ids.ActivityKind](ids.UUID(meeting.Id))
	reply, err := json.Marshal(map[string]any{"proposals": []map[string]any{{
		"summary": "Send the payment", "owner": "Ines Huber",
		"promiser_ref": "contact:" + customer.String(), "due_date": "",
		"source_lines": []int{1}, "confidence": taskWorthyConfidence, "commitment": openCommitmentMeaning("", ""),
	}}})
	if err != nil {
		t.Fatal(err)
	}
	done := e.read(t, cannedBrain{reply: string(reply)})
	if done.Status != activities.TranscriptReadDone {
		t.Fatalf("reading status: %s", done.Status)
	}
	if got := e.taskCount(t); got != 0 {
		t.Fatalf("customer promise created %d colleague tasks", got)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM conversation_claim WHERE kind = 'commitment_theirs' AND contact_id = $1`, customer); got != 1 {
		t.Fatalf("customer dependencies = %d, want one", got)
	}
}

func TestAnIncomingReportCannotAssignWorkToTheNamedColleague(t *testing.T) {
	e, reply := seedMailedPromise(t, "active")
	var payload extractPayload
	if err := json.Unmarshal([]byte(reply), &payload); err != nil {
		t.Fatal(err)
	}
	event := &(*payload.Events)[0]
	event.Quote = "You agreed to send the revised pricing by Friday."
	e.WsExec(t, `UPDATE activity SET direction='inbound', body=$2 WHERE id=$1`, event.MessageID, event.Quote)
	e.WsExec(t, `UPDATE activity_participant SET role=CASE WHEN user_id IS NOT NULL THEN 'to' ELSE 'from' END WHERE activity_id=$1`, event.MessageID)
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if err := runSignalPass(t, e, string(raw)); err != nil {
		t.Fatal(err)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM activity WHERE kind='task'`); got != 0 {
		t.Fatalf("reported promise created %d automatic tasks", got)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM approval WHERE kind='commitment_task' AND proposed_change->>'promiser_seat_id'=$1 AND on_behalf_of=$1::uuid`, e.Rep2.String()); got != 1 {
		t.Fatalf("review proposals preserving the named colleague=%d", got)
	}
}

func TestAMeetingReportCannotAssignWorkToTheNamedColleague(t *testing.T) {
	e := setupTranscript(t)
	e.WsExec(t, `UPDATE app_user SET display_name='Priya Raman' WHERE id=$1`, e.Rep2)
	body := "Mira: Priya Raman agreed to send the pricing by Friday."
	source := "transcript"
	meeting, _, err := e.Activities.LogActivity(e.ctx, activities.LogActivityInput{
		Kind: "meeting", Body: &body, SourceSystem: &source, Source: "manual",
		AttendeeUserIDs: []ids.UserID{ids.From[ids.UserKind](e.Rep2)},
	})
	if err != nil {
		t.Fatal(err)
	}
	e.activity = ids.From[ids.ActivityKind](ids.UUID(meeting.Id))
	reply, err := json.Marshal(map[string]any{"proposals": []map[string]any{{
		"summary": "Send pricing", "owner": "Priya Raman", "promiser_ref": "seat:" + e.Rep2.String(),
		"source_lines": []int{1}, "due_date": "", "confidence": 0.95, "commitment": openCommitmentMeaning("", ""),
	}}})
	if err != nil {
		t.Fatal(err)
	}
	done := e.read(t, cannedBrain{reply: string(reply)})
	if len(done.ProposalIDs) != 1 || e.taskCount(t) != 0 {
		t.Fatalf("reported promise produced %d proposals and %d tasks", len(done.ProposalIDs), e.taskCount(t))
	}
}
