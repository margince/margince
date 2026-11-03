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

// A mail with no reminder is the common case, and the pass says nothing about
// it.
//
// The consumer runs on every audience event, and most of those concern a mail
// no request was ever read out of.
//
// So this is the path it takes most often, and the one a failure would be
// loudest in.
func TestAMailWithNoReminderIsLeftAlone(t *testing.T) {
	o := setupOwed(t)
	o.contact(t, "Buyer", "buyer@customer.example")
	mail := o.capture(t, owedMail{
		from: "buyer@customer.example", to: o.seat, subject: "Thanks for the call",
		body: "Good to speak just now, nothing needed from you.", at: o.now.Add(-time.Hour),
		messageID: ids.NewV7().String() + "@customer.example",
	})
	if reminders := o.e.WsCount(t, `SELECT count(*) FROM activity
		WHERE source_system = $1 AND source_activity_id = $2`,
		activities.EmailRequestTaskSource, mail); reminders != 0 {
		t.Fatalf("%d reminders name a mail that asked for nothing", reminders)
	}

	o.e.WsExec(t, `UPDATE activity SET audience = 'participants' WHERE id = $1`, mail)
	if err := NewAudienceRescopeGen(o.e.Pool).HandleEvent(
		context.Background(), narrowedEvent(t, mail)); err != nil {
		t.Fatalf("narrowing a mail with no reminder answered %v", err)
	}
	if reminders := o.e.WsCount(t, `SELECT count(*) FROM activity
		WHERE source_system = $1 AND source_activity_id = $2`,
		activities.EmailRequestTaskSource, mail); reminders != 0 {
		t.Errorf("the pass invented %d reminders for a mail that had none", reminders)
	}
}

// A reminder a human set the audience of keeps it. ReasonManual is a lock no
// derivation moves, so a later change to the mail leaves the human's choice.
func TestAReminderAHumanNarrowedKeepsItsAudience(t *testing.T) {
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
	reminder, _ := reminderOf(t, o, mail)
	if _, err := o.e.Activities.SetAudience(o.reader(), ids.From[ids.ActivityKind](ids.MustParse(reminder)),
		activities.SetAudienceInput{Audience: "selected", Members: []activities.AudienceMember{
			{SubjectType: "user", SubjectID: o.owner},
		}}); err != nil {
		t.Fatalf("narrowing the reminder by hand: %v", err)
	}

	// The mail is narrowed and opened again. Neither move reaches the reminder.
	for _, audience := range []string{"participants", "workspace"} {
		o.e.WsExec(t, `UPDATE activity SET audience = $2 WHERE id = $1`, mail, audience)
		if err := NewAudienceRescopeGen(o.e.Pool).HandleEvent(
			context.Background(), narrowedEvent(t, mail)); err != nil {
			t.Fatal(err)
		}
		if got := o.e.WsScalar(t, `SELECT audience FROM activity WHERE id = $1`,
			ids.MustParse(reminder)); got != "selected" {
			t.Errorf("after the mail moved to %s, the reminder reads %q, want the human's selected", audience, got)
		}
	}
}
