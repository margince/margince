// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/qualifytasks"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

func TestHeldLeadMeetingsUseTheQualificationWriterAndDeduplicate(t *testing.T) {
	l := setupLeadMail(t)
	lead := l.ownedLead(t, "lena@prospect.example", l.e.Rep2)
	now := clocktest.Now(t)
	writer := newLeadWorkTasks(l.e.DB(), slog.Default())
	writer.now = func() time.Time { return now }
	store := l.e.Activities.WithLeadQualifyTasks(writer)
	for _, status := range []string{"booked", "held"} {
		meeting, _, err := store.LogActivity(l.e.Admin(), activities.LogActivityInput{
			Kind: "meeting", Subject: new("Lead demo"), OccurredAt: new(now.Add(-time.Hour)), MeetingStatus: &status, Source: "manual", CounterpartyEmail: "lena@prospect.example",
			Links: []activities.ActivityLinkInput{{EntityType: "lead", EntityID: lead.UUID}},
		})
		if err != nil {
			t.Fatal(err)
		}
		if status == "booked" {
			_, err = store.UpdateActivity(l.e.Admin(), ids.From[ids.ActivityKind](ids.UUID(meeting.Id)), activities.UpdateActivityInput{MeetingStatus: new("held")})
			if err != nil {
				t.Fatal(err)
			}
		}
		for range 2 {
			if _, err := store.UpdateActivity(l.e.Admin(), ids.From[ids.ActivityKind](ids.UUID(meeting.Id)), activities.UpdateActivityInput{MeetingStatus: new("held")}); err != nil {
				t.Fatal(err)
			}
		}
		if n := l.qualifyAsks(t, lead, l.e.Rep2); n != 1 {
			t.Fatalf("held meetings left %d open qualification tasks", n)
		}
	}
}

func TestLegacyQualificationRepairKeepsTheNewestAndTranslatesUneditedTitles(t *testing.T) {
	l := setupLeadMail(t)
	lead := l.ownedLead(t, "lena@prospect.example", l.e.Rep2)
	system := principal.SystemActing(l.e.Admin(), leadRepliedActor)
	tasks := legacyQualificationTasks(t, l, lead, 3)
	other := l.ownedLead(t, "mira@prospect.example", l.e.Rep2)
	edited := legacyQualificationTasks(t, l, other, 1)[0]
	_, err := l.e.Activities.UpdateActivity(l.e.Admin(), edited, activities.UpdateActivityInput{Subject: new("My qualification decision")})
	if err != nil {
		t.Fatal(err)
	}
	runMachineAddressTask(system, t, l.e, qualifytasks.TextDataTask())
	runMachineAddressTask(system, t, l.e, qualifytasks.DuplicatesDataTask(func() time.Time { return clocktest.Now(t) }))
	if n := l.qualifyAsks(t, lead, l.e.Rep2); n != 1 {
		t.Fatalf("repair left %d open tasks", n)
	}
	runMachineAddressTask(system, t, l.e, qualifytasks.TextDataTask())
	vi := systemtext.WithLocale(l.e.Admin(), textlang.Vietnamese)
	row, err := l.e.Activities.GetActivity(vi, tasks[2], storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if row.SystemDisplay == nil || row.SystemDisplay.Subject == nil || *row.SystemDisplay.Subject != "Xem xét điều kiện của Lena" {
		t.Fatalf("legacy title display=%+v", row.SystemDisplay)
	}
	human, err := l.e.Activities.GetActivity(vi, edited, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if human.SystemDisplay != nil && human.SystemDisplay.Subject != nil {
		t.Fatal("repair translated a human edit")
	}
	before := l.e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type='activity'`)
	runMachineAddressTask(system, t, l.e, qualifytasks.DuplicatesDataTask(func() time.Time { return clocktest.Now(t) }))
	if n := l.e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type='activity'`); n != before {
		t.Fatal("duplicate repair replay wrote more audit rows")
	}
}

func legacyQualificationTasks(t *testing.T, l *leadMailEnv, lead ids.LeadID, count int) []ids.ActivityID {
	t.Helper()
	writer := newLeadWorkTasks(l.e.DB(), slog.Default())
	system := principal.SystemActing(l.e.Admin(), leadRepliedActor)
	var tasks []ids.ActivityID
	for range count {
		source, _, err := l.e.Activities.LogActivity(l.e.Admin(), activities.LogActivityInput{Kind: "meeting", Subject: new("Demo"), OccurredAt: new(clocktest.Now(t).Add(-time.Hour)), MeetingStatus: new("held"), Source: "manual"})
		if err != nil {
			t.Fatal(err)
		}
		input := writer.repliedTaskInput(lead, ids.UUID(source.Id), "Review qualification for Lena", &l.e.Rep2)
		input.SystemText = nil
		task, _, err := l.e.Activities.LogActivity(system, input)
		if err != nil {
			t.Fatal(err)
		}
		tasks = append(tasks, ids.From[ids.ActivityKind](ids.UUID(task.Id)))
	}
	return tasks
}

func TestQualificationAndTextRepairsRollBackTheirWritesAndRejectInvalidCursors(t *testing.T) {
	for _, kind := range []string{"qualification", "text"} {
		t.Run(kind, func(t *testing.T) {
			l := setupLeadMail(t)
			lead := l.ownedLead(t, "lena@prospect.example", l.e.Rep2)
			legacyQualificationTasks(t, l, lead, 2)
			task := qualifytasks.DuplicatesDataTask(func() time.Time { return clocktest.Now(t) })
			if kind == "text" {
				task = qualifytasks.TextDataTask()
			}
			ctx := datatask.Acting(l.e.Admin(), task.ID)
			before := l.e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type='activity'`)
			events := l.e.WsCount(t, `SELECT count(*) FROM event_outbox`)
			original := task.Steps[0].Batch
			task.Steps[0].Limit = 1
			task.Steps[0].MinLimit = 1
			task.Steps[0].Batch = func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
				out, err := original(ctx, tx, in)
				if err != nil {
					return out, err
				}
				if out.Changed != 1 {
					t.Fatalf("batch changed %d rows", out.Changed)
				}
				return out, errors.New("stop after the domain writes")
			}
			runMachineAddressTask(ctx, t, l.e, task)
			if l.qualifyAsks(t, lead, l.e.Rep2) != 2 {
				t.Fatal("task completion escaped rollback")
			}
			if n := l.e.WsCount(t, `SELECT count(*) FROM activity WHERE system_text IS NOT NULL AND source_system='lead_replied'`); n != 0 {
				t.Fatal("descriptor escaped rollback")
			}
			if n := l.e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type='activity'`); n != before {
				t.Fatal("activity audit escaped rollback")
			}
			if n := l.e.WsCount(t, `SELECT count(*) FROM event_outbox`); n != events {
				t.Fatal("activity event escaped rollback")
			}
			if err := database.WithTx(ctx, l.e.Pool, func(tx pgx.Tx) error {
				_, err := original(ctx, tx, datatask.BatchIn{Limit: 1, Cursor: []byte(`"invalid"`)})
				return err
			}); err == nil {
				t.Fatal("repair accepted a malformed cursor")
			}
		})
	}
}

func TestLegacyQualificationTextDistinguishesAnUnnamedLeadFromAName(t *testing.T) {
	for _, name := range []string{"unnamed", "named"} {
		t.Run(name, func(t *testing.T) {
			l := setupLeadMail(t)
			system := principal.SystemActing(l.e.Admin(), leadRepliedActor)
			writer := newLeadWorkTasks(l.e.DB(), slog.Default())
			named := name == "named"
			input := contacts.CreateLeadInput{Source: "manual", LinkedInURL: new("https://www.linkedin.com/in/unnamed-lead")}
			expected := "Xem xét điều kiện của Một lead"
			if named {
				input.LinkedInURL = nil
				input.FullName = new("A lead")
				input.Email = new("named@prospect.example")
				expected = "Xem xét điều kiện của A lead"
			}
			lead, _, err := l.e.Contacts.CreateLead(l.e.Admin(), input)
			if err != nil {
				t.Fatal(err)
			}
			source, _, err := l.e.Activities.LogActivity(l.e.Admin(), activities.LogActivityInput{Kind: "meeting", Subject: new("Demo"), OccurredAt: new(clocktest.Now(t).Add(-time.Hour)), MeetingStatus: new("held"), Source: "manual"})
			if err != nil {
				t.Fatal(err)
			}
			taskInput := writer.repliedTaskInput(ids.From[ids.LeadKind](ids.UUID(lead.Id)), ids.UUID(source.Id), "Review qualification for A lead", &l.e.Rep2)
			taskInput.SystemText = nil
			task, _, err := l.e.Activities.LogActivity(system, taskInput)
			if err != nil {
				t.Fatal(err)
			}
			runMachineAddressTask(system, t, l.e, qualifytasks.TextDataTask())
			row, err := l.e.Activities.GetActivity(systemtext.WithLocale(l.e.Admin(), textlang.Vietnamese), ids.From[ids.ActivityKind](ids.UUID(task.Id)), storekit.LiveOnly)
			if err != nil {
				t.Fatal(err)
			}
			if row.SystemDisplay == nil || row.SystemDisplay.Subject == nil || *row.SystemDisplay.Subject != expected {
				t.Fatalf("named=%t: display=%+v, want %q", named, row.SystemDisplay, expected)
			}
		})
	}
}
