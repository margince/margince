// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// No task moves last_activity_at on a contact, company, deal or project. That
// holds whether a member logged it or an agent filed it from a mail.
// Mail, a meeting, a call and a note still do. Tasks land through the real
// writers: LogActivity for a member, writeCommitmentTask for the agent.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/projects"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// taskClockFixture is an account with a project, every record holding one
// mail sent three days ago.
type taskClockFixture struct {
	accountFixture
	project ids.UUID
	mailed  time.Time
}

func newTaskClockFixture(t *testing.T) taskClockFixture {
	t.Helper()
	f := taskClockFixture{accountFixture: newAccountFixture(t)}
	f.now = clocktest.Now(t).UTC().Truncate(time.Second)
	project, err := f.e.Projects.CreateProject(f.e.Admin(), projects.CreateProjectInput{
		Name: "Task clock rollout", CompanyID: ids.From[ids.CompanyKind](f.company), Source: "manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	f.project = ids.UUID(project.Id)
	f.mailed = f.now.AddDate(0, 0, -3)
	f.logOnAll(t, "email", f.mailed)
	return f
}

func (f taskClockFixture) links() []activities.ActivityLinkInput {
	return []activities.ActivityLinkInput{
		onRecord("contact", f.contact), onRecord("deal", f.deal), onRecord("project", f.project),
	}
}

// logOnAll logs one activity by hand on the contact, the deal and the project.
func (f taskClockFixture) logOnAll(t *testing.T, kind string, at time.Time) {
	t.Helper()
	subject := "Task clock " + kind
	in := activities.LogActivityInput{Kind: kind, Subject: &subject, OccurredAt: &at, Source: "manual", Links: f.links()}
	if kind == "email" || kind == "call" {
		in.Direction = new("inbound")
	}
	if _, _, err := f.e.Activities.LogActivity(f.e.Admin(), in); err != nil {
		t.Fatalf("logging the %s: %v", kind, err)
	}
}

// fileAgentTask files a promise's task the way the commitment scan does.
func (f taskClockFixture) fileAgentTask(t *testing.T) {
	t.Helper()
	ctx := withTranscriptReader(f.e.Admin(), "human:"+f.e.AdminUser.String(), ids.NewV7())
	source := f.e.WsScalar(t, `SELECT id::text FROM activity WHERE kind = 'email' LIMIT 1`)
	sourceID, err := ids.Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	locator := commitmentLocator(sourceID, claimKindOurs, "seat:"+f.e.AdminUser.String(), "1:send the pricing")
	if err := database.WithWorkspaceTx(ctx, f.e.Pool, func(tx pgx.Tx) error {
		_, err := writeCommitmentTask(ctx, tx, f.e.Activities, f.e.Contacts, commitmentTask{
			Extractor: transcriptProposalActor, Locator: locator, Summary: "Send the pricing", Body: "send the pricing",
			SourceActivityID: sourceID, Links: f.links(), Assignee: f.e.AdminUser, Origin: activities.OriginAgent,
		})
		return err
	}); err != nil {
		t.Fatalf("filing the agent's task: %v", err)
	}
}

func (f taskClockFixture) projectClock(t *testing.T) *time.Time {
	t.Helper()
	var at *time.Time
	if err := f.e.Pool.QueryRow(context.Background(),
		`SELECT last_activity_at FROM project WHERE id = $1`, f.project).Scan(&at); err != nil {
		t.Fatal(err)
	}
	return at
}

// assertAll holds every clock, and the contact and deal as their lists show
// them, to want.
func (f taskClockFixture) assertAll(t *testing.T, want time.Time, why string) {
	t.Helper()
	f.assertClocks(t, want, why)
	if at := f.projectClock(t); at == nil || !at.Equal(want) {
		t.Errorf("project last_activity_at = %v, want %v: %s", at, want, why)
	}
	var facts activities.ProjectActivityFacts
	if err := f.e.DB().Tx(f.e.Admin(), func(tx pgx.Tx) (err error) {
		facts, err = f.e.Activities.ProjectActivityFactsTx(f.e.Admin(), tx, ids.From[ids.ProjectKind](f.project), f.now)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if facts.LastActivityAt == nil || !facts.LastActivityAt.Equal(want) {
		t.Errorf("the project page shows last activity %v, want %v as the stored column: %s", facts.LastActivityAt, want, why)
	}
	listed, _, err := f.e.Contacts.ListContacts(f.e.Admin(), contacts.ListContactsInput{Sort: new("-last_activity_at")})
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range listed {
		if ids.UUID(row.Id) == f.contact && (row.LastActivityAt == nil || !row.LastActivityAt.Equal(want)) {
			t.Errorf("the contact list shows last_activity_at = %v, want %v as the detail view: %s", row.LastActivityAt, want, why)
		}
	}
}

func TestATaskAMemberLogsLeavesLastActivityAlone(t *testing.T) {
	f := newTaskClockFixture(t)
	f.logOnAll(t, "task", f.now.Add(-time.Minute))
	f.assertAll(t, f.mailed, "a member's task is intent, not contact")
}

func TestATaskAnAgentFilesLeavesLastActivityAlone(t *testing.T) {
	f := newTaskClockFixture(t)
	f.fileAgentTask(t)
	f.assertAll(t, f.mailed, "the commitment scan's task is intent, not contact")
}

func TestANoteACallAndAMeetingStillMoveLastActivity(t *testing.T) {
	for i, kind := range []string{"note", "call", "meeting"} {
		t.Run(kind, func(t *testing.T) {
			f := newTaskClockFixture(t)
			at := f.now.Add(-time.Duration(i+1) * time.Hour)
			f.logOnAll(t, kind, at)
			f.assertAll(t, at, "a "+kind+" is contact")
		})
	}
}

// stampOldRule stores what the rule before 1792846717 gave every record: the
// newest linked activity of any kind, here the task.
func (f taskClockFixture) stampOldRule(t *testing.T, at time.Time) {
	t.Helper()
	for _, table := range []string{"contact", "company", "deal", "project"} {
		f.e.WsExec(t, `UPDATE `+pgx.Identifier{table}.Sanitize()+` SET last_activity_at = $1 WHERE id = ANY($2)`,
			at, []ids.UUID{f.contact, f.company, f.deal, f.project})
	}
}

func (f taskClockFixture) runTaskClockTask(ctx context.Context, t *testing.T, task datatask.Task) {
	t.Helper()
	now := clocktest.Now(t)
	engine, err := datatask.New(f.e.Pool, []datatask.Task{task}, "test", func() time.Time { return now },
		func(context.Context, pgx.Tx, datatask.Progress) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.Seed(ctx); err != nil {
		t.Fatal(err)
	}
	for range 20 {
		if _, err := engine.Execute(ctx); err != nil {
			t.Fatal(err)
		}
	}
}

func TestTheDataTaskTakesATaskOutOfAStoredClockOnce(t *testing.T) {
	f := newTaskClockFixture(t)
	tasked := f.now.Add(-time.Minute)
	f.logOnAll(t, "task", tasked)
	f.stampOldRule(t, tasked)
	ctx := principal.SystemActing(f.e.Admin(), "system:datatask-test")

	task := taskActivityClocksDataTask()
	for i := range task.Steps {
		task.Steps[i].Limit, task.Steps[i].MinLimit = 1, 1
	}
	f.runTaskClockTask(ctx, t, task)
	f.assertAll(t, f.mailed, "the repaired clock falls back to the newest mail")

	before := map[string]clockRow{}
	for table, id := range map[string]ids.UUID{"contact": f.contact, "company": f.company, "deal": f.deal, "project": f.project} {
		before[table] = readClockRow(t, f.e, table, id)
	}
	runCtx := datatask.Acting(principal.WithWorkspaceID(context.Background(), f.e.WS), task.ID)
	f.assertNoTaskClocks(t, "after the run")
	for _, step := range task.Steps {
		in := datatask.BatchIn{Limit: 10}
		err := f.e.DB().Tx(runCtx, func(tx pgx.Tx) error {
			out, err := step.Batch(runCtx, tx, in)
			if out.Changed != 0 {
				t.Errorf("a replay of step %s changed %d records, want none", step.Name, out.Changed)
			}
			return err
		})
		if err != nil {
			t.Fatalf("replaying step %s: %v", step.Name, err)
		}
	}
	for table, id := range map[string]ids.UUID{"contact": f.contact, "company": f.company, "deal": f.deal, "project": f.project} {
		if after := readClockRow(t, f.e, table, id); after != before[table] {
			t.Errorf("%s: the replay rewrote the row (%+v → %+v), want it untouched", table, before[table], after)
		}
	}
}

// A task and a mail in one second leave a clock the current rule also gives,
// so the task does not select it.
func TestTheDataTaskLeavesAClockATaskSharesWithAMail(t *testing.T) {
	f := newTaskClockFixture(t)
	f.logOnAll(t, "task", f.mailed)
	f.assertNoTaskClocks(t, "the stored value is the mail's")
}

// assertNoTaskClocks holds the data task's selection empty on every table.
func (f taskClockFixture) assertNoTaskClocks(t *testing.T, why string) {
	t.Helper()
	ctx := principal.WithWorkspaceID(context.Background(), f.e.WS)
	for _, table := range []storekit.ClockTable{storekit.ClockContacts, storekit.ClockCompanies, storekit.ClockDeals, storekit.ClockProjects} {
		var count int64
		if err := database.WithWorkspaceTx(ctx, f.e.Pool, func(tx pgx.Tx) (err error) {
			count, err = storekit.CountTaskClocks(ctx, tx, table, 10)
			return err
		}); err != nil || count != 0 {
			t.Errorf("%s: %d clocks selected (%v), want none: %s", table, count, err, why)
		}
	}
}

func TestTheDataTaskRollsItsClocksBackWithItsProgress(t *testing.T) {
	f := newTaskClockFixture(t)
	tasked := f.now.Add(-time.Minute)
	f.logOnAll(t, "task", tasked)
	f.stampOldRule(t, tasked)
	ctx := principal.SystemActing(f.e.Admin(), "system:datatask-test")

	task := taskActivityClocksDataTask()
	for i := range task.Steps {
		original := task.Steps[i].Batch
		task.Steps[i].Batch = func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
			out, err := original(ctx, tx, in)
			if err != nil {
				return out, err
			}
			return out, errors.New("transaction interrupted after domain writes")
		}
	}
	f.runTaskClockTask(ctx, t, task)
	f.assertClocks(t, tasked, "a rolled-back batch keeps the stored clock")
	if _, err := datatask.UUIDCursor([]byte(`"not-a-uuid"`)); err == nil {
		t.Error("a malformed cursor was read as a position")
	}
}
