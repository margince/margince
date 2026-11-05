// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/attention"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

func TestLeadReplyAdviceFollowsTheSourceRatherThanTheImport(t *testing.T) {
	l := setupLeadMail(t)
	lead := l.ownedLead(t, "lena@prospect.example", l.e.Rep2)
	now := l.at.Add(5 * time.Minute)
	l.mail(t, owedMail{from: l.seat, to: "lena@prospect.example", subject: "Old enquiry", messageID: "old-root@mail.example", at: now.AddDate(0, 0, -32)}, true)
	l.mail(t, owedMail{from: l.seat, to: "lena@prospect.example", subject: "Current enquiry", messageID: "current-root@mail.example", at: l.at.Add(-time.Minute)}, true)
	old := l.mail(t, owedMail{from: "lena@prospect.example", to: l.seat, subject: "Old enquiry", inReplyTo: "old-root@mail.example", at: now.AddDate(0, 0, -31)}, true)
	if n := l.qualifyAsks(t, lead, l.e.Rep2); n != 0 {
		t.Fatalf("history manufactured %d tasks", n)
	}
	inbound := l.mail(t, owedMail{from: "lena@prospect.example", to: l.seat, subject: "Current enquiry", messageID: "current@prospect.example", inReplyTo: "current-root@mail.example", at: l.at}, true)
	assertCurrentLeadAdvice(t, l, lead, now, 1)
	assertLeadAdviceOnWorklist(t, l, now, 1)
	var due *time.Time
	var source *ids.UUID
	if err := database.WithWorkspaceTx(l.e.Admin(), l.e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT due_at, source_activity_id FROM activity WHERE source_system=$1 AND source_id=$2`, provenance.LeadRepliedSource, inbound.String()).Scan(&due, &source)
	}); err != nil {
		t.Fatal(err)
	}
	if due != nil || source == nil || *source != inbound {
		t.Fatalf("derived task deadline/source: %v %v", due, source)
	}
	answer := l.mail(t, owedMail{from: l.seat, to: "lena@prospect.example", subject: "Re: Current enquiry", inReplyTo: "current@prospect.example", at: l.at.Add(time.Minute)}, true)
	assertCurrentLeadAdvice(t, l, lead, now, 0)
	assertLeadAdviceOnWorklist(t, l, now, 0)
	l.mail(t, owedMail{from: "lena@prospect.example", to: l.seat, subject: "Re: Current enquiry", inReplyTo: "current@prospect.example", at: l.at.Add(2 * time.Minute)}, true)
	assertCurrentLeadAdvice(t, l, lead, now, 1)
	assertLeadAdviceOnWorklist(t, l, now, 1)
	if _, err := l.e.Activities.ArchiveActivity(l.e.Admin(), ids.From[ids.ActivityKind](answer), nil); err != nil {
		t.Fatal(err)
	}
	assertCurrentLeadAdvice(t, l, lead, now, 1)
	assertLeadAdviceOnWorklist(t, l, now, 1)
	if !l.filedUnderLead(t, old, lead) {
		t.Fatal("historical source was lost")
	}
}

func assertCurrentLeadAdvice(t *testing.T, l *leadMailEnv, lead ids.LeadID, now time.Time, want int) {
	t.Helper()
	assignee := ids.From[ids.UserKind](l.e.Rep2)
	in := activities.ListActivitiesInput{AssigneeID: &assignee, EntityType: new("lead"), EntityID: &lead.UUID, WorklistAsOf: &now}
	store := l.e.Activities.WithClock(func() time.Time { return now })
	rows, _, err := store.ListActivities(l.e.Admin(), in)
	if err != nil {
		t.Fatal(err)
	}
	count, err := store.CountActivities(l.e.Admin(), in)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != want || count != want {
		t.Fatalf("current advice: rows=%d count=%d want=%d", len(rows), count, want)
	}

}

func TestLeadReplyAdviceRejectsFutureMailAndSharesLegacyAdmission(t *testing.T) {
	l := setupLeadMail(t)
	lead := l.ownedLead(t, "lena@prospect.example", l.e.Rep2)
	now := l.at.Add(5 * time.Minute)
	l.mail(t, owedMail{from: "lena@prospect.example", to: l.seat, subject: "Postdated", at: now.Add(time.Hour)}, true)
	if n := l.qualifyAsks(t, lead, l.e.Rep2); n != 0 {
		t.Fatalf("future mail manufactured %d tasks", n)
	}
	source := l.mail(t, owedMail{from: "lena@prospect.example", to: l.seat, subject: "Legacy source", at: now.AddDate(0, 0, -31)}, true)
	writer := newLeadWorkTasks(InstallationDB(l.e.Pool), slog.Default())
	writer.now = func() time.Time { return now }
	input := writer.repliedTaskInput(lead, source, "Historic qualification", &l.e.Rep2)
	input.SourceActivityID = nil
	input.DueAt = &now
	if _, _, err := l.e.Activities.LogActivity(principal.SystemActing(l.e.Admin(), leadRepliedActor), input); err != nil {
		t.Fatal(err)
	}
	assertCurrentLeadAdvice(t, l, lead, now, 0)
	if err := database.WithWorkspaceTx(l.e.Admin(), l.e.Pool, func(tx pgx.Tx) error {
		return writer.FileLeadReplyTx(l.e.Admin(), tx, lead, source)
	}); err != nil {
		t.Fatal(err)
	}
	if n := l.qualifyAsks(t, lead, l.e.Rep2); n != 1 {
		t.Fatalf("replay created another historical task: %d", n)
	}
}

func TestLeadQualificationKeepsBookingsDistinctFromMail(t *testing.T) {
	l := setupLeadMail(t)
	now := l.at.Add(5 * time.Minute)
	writer := newLeadWorkTasks(InstallationDB(l.e.Pool), slog.Default())
	writer.now = func() time.Time { return now }
	for _, tc := range []struct {
		name, kind, direction, status string
		offset                        time.Duration
		want                          int
	}{
		{"boundary", "email", "inbound", "", -30 * 24 * time.Hour, 1},
		{"past boundary", "email", "inbound", "", -30*24*time.Hour - time.Second, 0},
		{"future email", "email", "inbound", "", time.Hour, 0},
		{"our email", "email", "outbound", "", -time.Hour, 0},
		{"future booking", "meeting", "", "booked", 24 * time.Hour, 1},
		{"canceled booking", "meeting", "", "canceled", 24 * time.Hour, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lead := l.ownedLead(t, ids.NewV7().String()+"@prospect.example", l.e.Rep2)
			at := now.Add(tc.offset)
			in := activities.LogActivityInput{Kind: tc.kind, Subject: &tc.name, OccurredAt: &at, Source: "manual", Links: []activities.ActivityLinkInput{{EntityType: "lead", EntityID: lead.UUID}}}
			if tc.direction != "" {
				in.Direction = &tc.direction
			}
			if tc.status != "" {
				in.MeetingStatus = &tc.status
			}
			source, _, err := l.e.Activities.LogActivity(l.e.Admin(), in)
			if err != nil {
				t.Fatal(err)
			}
			if err := database.WithWorkspaceTx(l.e.Admin(), l.e.Pool, func(tx pgx.Tx) error {
				if tc.kind == "meeting" {
					return writer.FileLeadBookingTx(l.e.Admin(), tx, lead, ids.UUID(source.Id))
				}
				return writer.FileLeadReplyTx(l.e.Admin(), tx, lead, ids.UUID(source.Id))
			}); err != nil {
				t.Fatal(err)
			}
			assertCurrentLeadAdvice(t, l, lead, now, tc.want)
		})
	}
}

func TestLegacyNotificationDatesAreNotDeadlinesAndHumanDatesSurvive(t *testing.T) {
	l := setupLeadMail(t)
	lead := l.ownedLead(t, "lena@prospect.example", l.e.Rep2)
	now := l.at.Add(5 * time.Minute)
	source := l.mail(t, owedMail{from: "lena@prospect.example", to: l.seat, subject: "Source", at: l.at}, true)
	writer := newLeadWorkTasks(InstallationDB(l.e.Pool), slog.Default())
	writer.now = func() time.Time { return now }
	in := writer.repliedTaskInput(lead, source, "Choose qualification", &l.e.Rep2)
	in.SourceActivityID = nil
	in.DueAt = &now
	task, _, err := l.e.Activities.LogActivity(principal.SystemActing(l.e.Admin(), leadRepliedActor), in)
	if err != nil {
		t.Fatal(err)
	}
	if task.DueAt != nil {
		t.Fatalf("notification timestamp became a deadline: %v", task.DueAt)
	}
	taskID := ids.From[ids.ActivityKind](ids.UUID(task.Id))
	task, err = l.e.Activities.UpdateActivity(l.e.Admin(), taskID, activities.UpdateActivityInput{Body: new("Keep this qualification decision")})
	if err != nil {
		t.Fatal(err)
	}
	if task.DueAt != nil {
		t.Fatalf("body edit revived a generated deadline: %v", task.DueAt)
	}
	due := now.Add(24 * time.Hour)
	task, err = l.e.Activities.UpdateActivity(l.e.Admin(), taskID, activities.UpdateActivityInput{DueAt: &due})
	if err != nil {
		t.Fatal(err)
	}
	if task.DueAt == nil || !task.DueAt.Equal(due) {
		t.Fatalf("human deadline lost: %v", task.DueAt)
	}
	assertCurrentLeadAdvice(t, l, lead, now.AddDate(0, 0, 31), 1)
}

func TestUndatedContactArrivalStillReachesItsOwnersWorklist(t *testing.T) {
	l := setupLeadForContact(t)
	reader := l.e.As(l.e.Rep1, nil, integration.AdminPerms)
	_, _, err := l.store.CreateLead(reader, contacts.CreateLeadInput{FullName: new("Contact submission"), Email: &l.address, Source: "web_form"})
	if held := requireContactHeld(t, err, "submission"); held.Field != "email" {
		t.Fatalf("the submission was held on %q, want the contact's email", held.Field)
	}
	owner := l.e.As(l.e.Rep2, nil, integration.AdminPerms)
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	tasks := attentionTasks{store: l.e.Activities.WithClock(func() time.Time { return now })}
	rows, err := tasks.OpenForViewer(owner, now, now.Add(24*time.Hour), 100, attention.TasksMine, ids.Nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	total, err := tasks.CountOpenForViewer(owner, now, now.Add(24*time.Hour), attention.TasksMine, ids.Nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || total != 1 {
		t.Fatalf("undated submission: rows=%d count=%d", len(rows), total)
	}
}

func TestAdoptedLegacyLeadAdviceSurvivesItsSourceArchive(t *testing.T) {
	l := setupLeadMail(t)
	lead := l.ownedLead(t, "lena@prospect.example", l.e.Rep2)
	now := l.at.Add(5 * time.Minute)
	source := l.mail(t, owedMail{from: "lena@prospect.example", to: l.seat, subject: "Current source", at: l.at}, true)
	writer := newLeadWorkTasks(InstallationDB(l.e.Pool), slog.Default())
	writer.now = func() time.Time { return now }
	in := writer.repliedTaskInput(lead, source, "Review qualification", &l.e.Rep2)
	in.SourceActivityID = nil
	task, _, err := l.e.Activities.LogActivity(principal.SystemActing(l.e.Admin(), leadRepliedActor), in)
	if err != nil {
		t.Fatal(err)
	}
	if err := database.WithWorkspaceTx(l.e.Admin(), l.e.Pool, func(tx pgx.Tx) error { return writer.FileLeadReplyTx(l.e.Admin(), tx, lead, source) }); err != nil {
		t.Fatal(err)
	}
	if n := l.qualifyAsks(t, lead, l.e.Rep2); n != 1 {
		t.Fatalf("legacy task did not deduplicate: %d", n)
	}
	taskID := ids.From[ids.ActivityKind](ids.UUID(task.Id))
	if _, err := l.e.Activities.UpdateActivity(l.e.Admin(), taskID, activities.UpdateActivityInput{Subject: new("My qualification decision")}); err != nil {
		t.Fatal(err)
	}
	if _, err := l.e.Activities.ArchiveActivity(l.e.Admin(), ids.From[ids.ActivityKind](source), nil); err != nil {
		t.Fatal(err)
	}
	assertCurrentLeadAdvice(t, l, lead, now, 1)
}

func TestLeadAdviceFollowsItsSourceAudienceOnEveryRead(t *testing.T) {
	l := setupLeadMail(t)
	l.at = l.at.Add(-2 * time.Hour)
	lead := l.ownedLead(t, "lena@prospect.example", l.e.Rep2)
	now := l.at.Add(5 * time.Minute)
	writer := newLeadWorkTasks(InstallationDB(l.e.Pool), slog.Default())
	writer.now = func() time.Time { return now }
	source, _, err := l.e.Activities.LogActivity(l.e.Admin(), activities.LogActivityInput{Kind: "email", Subject: new("Qualification source"), Direction: new("inbound"), OccurredAt: &l.at, Source: "manual", Links: []activities.ActivityLinkInput{{EntityType: "lead", EntityID: lead.UUID}}})
	if err != nil {
		t.Fatal(err)
	}
	sourceID := ids.From[ids.ActivityKind](ids.UUID(source.Id))
	if err := database.WithWorkspaceTx(l.e.Admin(), l.e.Pool, func(tx pgx.Tx) error { return writer.FileLeadReplyTx(l.e.Admin(), tx, lead, sourceID.UUID) }); err != nil {
		t.Fatal(err)
	}
	assertCurrentLeadAdvice(t, l, lead, now, 1)
	before, err := l.e.Contacts.GetLead(l.e.Admin(), lead, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if before.NextTaskSubject == nil {
		t.Fatal("fixture has no visible lead task before narrowing")
	}
	if _, err := l.e.Activities.SetAudience(l.e.Admin(), sourceID, activities.SetAudienceInput{Audience: "selected", Members: []activities.AudienceMember{{SubjectType: "user", SubjectID: l.e.AdminUser}}}); err != nil {
		t.Fatal(err)
	}
	reader := l.e.As(l.e.Rep2, nil, integration.AdminPerms)
	tasks := attentionTasks{store: l.e.Activities.WithClock(func() time.Time { return now })}
	rows, err := tasks.OpenForViewer(reader, now, now.Add(24*time.Hour), 100, attention.TasksMine, ids.Nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Fatal("task exposed a source whose audience changed")
	}
	card, err := l.e.Contacts.GetLead(l.e.Admin(), lead, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if card.NextTaskSubject != nil {
		t.Fatal("lead summary exposed a task derived from private source mail")
	}
	next, _, err := l.e.Activities.LogActivity(l.e.Admin(), activities.LogActivityInput{Kind: "email", Subject: new("New shared qualification source"), Direction: new("inbound"), OccurredAt: new(l.at.Add(time.Minute)), Source: "manual", Links: []activities.ActivityLinkInput{{EntityType: "lead", EntityID: lead.UUID}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.WithWorkspaceTx(l.e.Admin(), l.e.Pool, func(tx pgx.Tx) error { return writer.FileLeadReplyTx(l.e.Admin(), tx, lead, ids.UUID(next.Id)) }); err != nil {
		t.Fatal(err)
	}
	assertLeadAdviceOnWorklist(t, l, now, 1)
	if n := l.qualifyAsks(t, lead, l.e.Rep2); n != 2 {
		t.Fatalf("unreadable old advice blocked a new readable source: %d", n)
	}
}

func assertLeadAdviceOnWorklist(t *testing.T, l *leadMailEnv, now time.Time, want int) {
	t.Helper()
	reader := l.e.As(l.e.Rep2, nil, integration.AdminPerms)
	tasks := attentionTasks{store: l.e.Activities.WithClock(func() time.Time { return now })}
	due, err := tasks.OpenForViewer(reader, now, now.Add(24*time.Hour), 100, attention.TasksMine, ids.Nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	total, err := tasks.CountOpenForViewer(reader, now, now.Add(24*time.Hour), attention.TasksMine, ids.Nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(due) != want || total != want {
		t.Fatalf("worklist: rows=%d count=%d want=%d", len(due), total, want)
	}
}
