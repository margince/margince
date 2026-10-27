// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// reminderOf is the open request reminder read out of the mail, with the reach
// it currently carries.
func reminderOf(t *testing.T, o *owedEnv, mail ids.UUID) (string, string) {
	t.Helper()
	id := o.e.WsScalar(t, `SELECT id::text FROM activity WHERE source_system = $1
		AND source_activity_id = $2`, activities.EmailRequestTaskSource, mail)
	return id, o.e.WsScalar(t, `SELECT audience FROM activity WHERE id = $1`, ids.MustParse(id))
}

// A reminder's reach follows its mail's, either way.
//
// The mail's audience is derived, and derived again later, so one read at
// filing goes stale. A narrowed mail then keeps a reminder that hides it from
// the waiting lane. An opened one keeps a reminder its assignee alone reads.
func TestAReminderFollowsTheReachOfItsMail(t *testing.T) {
	o := setupOwed(t)
	o.contact(t, "Buyer", "buyer@customer.example")
	mail := o.capture(t, owedMail{
		from: "buyer@customer.example", to: o.seat, subject: "Proposal for the integration project",
		body: "Please send the proposal for our integration project.", at: o.now.Add(-time.Hour),
		messageID: ids.NewV7().String() + "@customer.example",
	})
	if _, err := o.e.Activities.SetCaptureLabel(o.judge(), mail, "commitment"); err != nil {
		t.Fatal(err)
	}
	runOwedWorker(t, o.e, &owedBrainStub{verdict: activities.OwedVerdictAsksUs, confidence: .99})
	reminder, reach := reminderOf(t, o, mail)
	if reach != "workspace" {
		t.Fatalf("the reminder of workspace mail reads %q, so the moves below prove nothing", reach)
	}

	// The mail is narrowed, and the consumer that answers an audience change
	// runs. The reminder follows it down.
	o.e.WsExec(t, `UPDATE activity SET audience = 'participants' WHERE id = $1`, mail)
	if err := NewAudienceRescopeGen(o.e.Pool).HandleEvent(
		context.Background(), narrowedEvent(t, mail)); err != nil {
		t.Fatal(err)
	}
	if got := o.e.WsScalar(t, `SELECT audience FROM activity WHERE id = $1`,
		ids.MustParse(reminder)); got != "selected" {
		t.Errorf("the narrowed mail's reminder reads %q, want it narrowed too", got)
	}

	// And back up: a mail opened to the workspace leaves no reminder that only
	// its assignee reads.
	o.e.WsExec(t, `UPDATE activity SET audience = 'workspace' WHERE id = $1`, mail)
	if err := NewAudienceRescopeGen(o.e.Pool).HandleEvent(
		context.Background(), narrowedEvent(t, mail)); err != nil {
		t.Fatal(err)
	}
	if got := o.e.WsScalar(t, `SELECT audience FROM activity WHERE id = $1`,
		ids.MustParse(reminder)); got != "workspace" {
		t.Errorf("the opened mail's reminder reads %q, want it opened too", got)
	}
}
