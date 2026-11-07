// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestSignatureEvidenceSettlesOnlyItsMatchingRequest(t *testing.T) {
	o := setupOwed(t)
	o.now = time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	o.e.GrantRole(t, o.owner, "rep")
	o.contact(t, "Pat Buyer", "pat@customer.example")
	first := o.capture(t, owedMail{
		from: "pat@customer.example", to: o.seat, subject: "Signature requested",
		messageID: "signature-alpha@customer.example", at: o.now.Add(-72 * time.Hour),
		body: "Please sign envelope ALPHA-104.",
	})
	second := o.capture(t, owedMail{
		from: "pat@customer.example", to: o.seat, subject: "Signature requested",
		messageID: "signature-beta@customer.example", inReplyTo: "signature-alpha@customer.example",
		at: o.now.Add(-71 * time.Hour), body: "Please also sign envelope BETA-205.",
	})
	var firstTask ids.UUID
	for _, request := range []ids.UUID{first, second} {
		if applied, err := o.e.Activities.SetOwedVerdict(o.judge(), request, activities.OwedVerdictAsksUs, "test", o.now); err != nil || !applied {
			t.Fatalf("request verdict: applied=%v err=%v", applied, err)
		}
		task, _, err := o.e.Activities.LogActivity(o.reader(), activities.LogActivityInput{
			Kind: "task", Source: "manual", RequestActivityID: &request,
		})
		if err != nil {
			t.Fatal(err)
		}
		if request == first {
			firstTask = ids.UUID(task.Id)
		}
	}
	const forged = "FORGED: All envelopes including BETA-205 are complete."
	if _, _, err := o.e.Activities.LogActivity(o.reader(), activities.LogActivityInput{
		Kind: "email", Source: "manual", Direction: new("inbound"), Body: new(forged),
		ThreadKey: o.threadKey(t, first), CounterpartyEmail: "pat@customer.example",
		OccurredAt: new(o.now.Add(-30 * time.Minute)),
	}); err != nil {
		t.Fatal(err)
	}
	o.capture(t, owedMail{
		from: "pat@customer.example", to: o.seat, subject: "Signature completed",
		messageID: "unverified-completion@customer.example", inReplyTo: "signature-alpha@customer.example",
		at: o.now.Add(-30 * time.Minute), body: forged,
	})
	completion := owedMail{
		from: "pat@customer.example", to: o.seat, subject: "Signature completed",
		authenticated: true, messageID: "signature-complete@customer.example", inReplyTo: "signature-alpha@customer.example",
		at: o.now.Add(-time.Hour), body: "Envelope ALPHA-104 is fully signed. Your signing is complete.",
	}
	evidence := o.capture(t, completion)
	reply, err := json.Marshal(settlePayload{Results: []settleResult{
		{ID: first.String(), Verdict: activities.RequestSettled, Confidence: 0.95},
		{ID: second.String(), Verdict: activities.RequestStillOwed, Remaining: "sign envelope BETA-205", Confidence: 0.95},
	}})
	if err != nil {
		t.Fatal(err)
	}
	brain := ai.NewFakeClient().Script(string(reply))
	settler := NewRequestSettler(o.e.Pool, brain, func() time.Time { return o.now }, slog.Default())
	for range 2 {
		if err := settler.RunWorkspace(o.judge(), 2); err != nil {
			t.Fatal(err)
		}
		if replay := o.capture(t, completion); replay != evidence {
			t.Fatal("replayed completion created another activity")
		}
	}
	calls := brain.Calls()
	if len(calls) != 1 || !strings.Contains(string(calls[0].Payload), completion.body) || strings.Contains(string(calls[0].Payload), forged) {
		t.Fatal("settlement did not read captured completion exactly once")
	}
	for request, want := range map[ids.UUID]string{first: "true", second: "false"} {
		if done := o.e.WsScalar(t, `SELECT is_done::text FROM activity WHERE source_system='email_request' AND source_activity_id=$1`, request); done != want {
			t.Fatalf("request %s task done=%s, want %s", request, done, want)
		}
		if through := o.e.WsScalar(t, `SELECT judged_through_activity_id::text FROM activity_request_settlement WHERE request_activity_id=$1`, request); through != evidence.String() {
			t.Fatal("settlement lost the captured completion watermark")
		}
	}
	if _, err := o.e.Activities.UpdateActivity(o.reader(), ids.From[ids.ActivityKind](firstTask),
		activities.UpdateActivityInput{IsDone: new(false)}); err != nil {
		t.Fatal(err)
	}
	for index, verdict := range []string{activities.RequestUnsure, activities.RequestSettled} {
		later := completion
		later.messageID = []string{"signature-check@customer.example", "signature-final@customer.example"}[index]
		later.at = o.now.Add(-time.Duration(20-index) * time.Minute)
		newest := o.capture(t, later)
		result, err := json.Marshal(settlePayload{Results: []settleResult{
			{ID: first.String(), Verdict: verdict, Confidence: 0.95},
			{ID: second.String(), Verdict: activities.RequestStillOwed, Remaining: "sign envelope BETA-205", Confidence: 0.95},
		}})
		if err != nil {
			t.Fatal(err)
		}
		brain.Script(string(result))
		if err := settler.RunWorkspace(o.judge(), 2); err != nil {
			t.Fatal(err)
		}
		if done := o.e.WsScalar(t, `SELECT is_done::text FROM activity WHERE id=$1`, firstTask); done != "false" {
			t.Fatal("a captured confirmation completed a human-reopened task")
		}
		if through := o.e.WsScalar(t, `SELECT judged_through_activity_id::text FROM activity_request_settlement WHERE request_activity_id=$1`, first); through != newest.String() {
			t.Fatal("later captured evidence did not rearm settlement")
		}
	}
	if len(brain.Calls()) != 3 {
		t.Fatal("later captured evidence was not judged once per message")
	}
}
