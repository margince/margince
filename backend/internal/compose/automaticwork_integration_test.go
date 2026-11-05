// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/workflow"
)

type workFixture struct {
	e         *integration.Env
	approvals *approvals.Service
	source    ids.UUID
}

func newWorkFixture(t *testing.T) *workFixture {
	t.Helper()
	e := integration.Setup(t)
	meeting, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{Kind: "meeting", Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	return &workFixture{e: e, approvals: approvalsServiceWithEffects(e.Pool), source: ids.UUID(meeting.Id)}
}

// promiseTask files a task for a promise the way the extractor does.
func (f *workFixture) promiseTask(t *testing.T, assignee ids.UUID) ids.ActivityID {
	t.Helper()
	ctx := f.e.AutomationCtx(assignee)
	var task ids.UUID
	if err := database.WithWorkspaceTx(ctx, f.e.Pool, func(tx pgx.Tx) error {
		var err error
		task, err = writeCommitmentTask(ctx, tx, f.e.Activities, f.e.Contacts, commitmentTask{
			Extractor: transcriptProposalActor, Locator: ids.NewV7().String(), Summary: "Send the pricing",
			SourceActivityID: f.source, Assignee: assignee,
		})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return ids.From[ids.ActivityKind](task)
}

// replyReminder files a reminder to answer a message, as the owed-verdict pass
// files one for the seat the message was addressed to.
func (f *workFixture) replyReminder(t *testing.T, assignee ids.UUID) ids.ActivityID {
	t.Helper()
	base := principal.WithCorrelationID(principal.WithWorkspaceID(context.Background(), f.e.WS), ids.NewV7())
	ctx := principal.WithActor(base, principal.Principal{
		Type: principal.PrincipalSystem, ID: activities.OwedVerdictCapturedBy,
	})
	sourceSystem, sourceID, owner := activities.EmailRequestTaskSource, ids.NewV7().String(), ids.From[ids.UserKind](assignee)
	task, _, err := f.e.Activities.LogActivity(ctx, activities.LogActivityInput{
		Kind: "task", Subject: new("Reply to email"), AssigneeID: &owner,
		SourceSystem: &sourceSystem, SourceID: &sourceID, SourceActivityID: &f.source,
		Source: activities.EmailRequestTaskSource, Origin: activities.OriginSystemRemediation,
	})
	if err != nil {
		t.Fatal(err)
	}
	return ids.From[ids.ActivityKind](ids.UUID(task.Id))
}

// approvalRequest stages a proposal the way an automation firing does, so it
// waits on `decider`.
func (f *workFixture) approvalRequest(t *testing.T, decider ids.UUID) ids.ApprovalID {
	t.Helper()
	id, err := f.approvals.Stage(f.e.AutomationCtx(decider), approvals.StageInput{
		Kind: string(workflow.ActionEmitFlowEvent), ProposedChange: json.RawMessage(`{"note":"needs a human"}`),
		DiffHash: "work-" + ids.NewV7().String(), Summary: "automation asks", TargetType: "contact",
		TargetID: f.e.SeedContact(t, "Asked About "+ids.NewV7().String(), nil),
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// dbNow is the database's clock, the one every writer stamps audit rows with,
// so the report's window is measured against the instants the items carry.
func (f *workFixture) dbNow(t *testing.T) time.Time {
	t.Helper()
	var now time.Time
	if err := f.e.Pool.QueryRow(context.Background(), `SELECT now()`).Scan(&now); err != nil {
		t.Fatal(err)
	}
	return now.UTC()
}

func (f *workFixture) seat(user ids.UUID) context.Context {
	return f.e.As(user, nil, integration.AdminPerms)
}

func (f *workFixture) update(t *testing.T, task ids.ActivityID, in activities.UpdateActivityInput) {
	t.Helper()
	if _, err := f.e.Activities.UpdateActivity(f.e.Admin(), task, in); err != nil {
		t.Fatal(err)
	}
}

func (f *workFixture) report(t *testing.T, now time.Time, days int) crmcontracts.AutomaticWorkHealth {
	t.Helper()
	ctx := f.e.Admin()
	var out crmcontracts.AutomaticWorkHealth
	if err := database.WithWorkspaceTx(ctx, f.e.Pool, func(tx pgx.Tx) error {
		var err error
		out, err = readAutomaticWorkHealth(ctx, tx, now, days)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return out
}

func tallyOf(t *testing.T, health crmcontracts.AutomaticWorkHealth, user ids.UUID, kind crmcontracts.AutomaticWorkKindName) (crmcontracts.AutomaticWorkColleague, crmcontracts.AutomaticWorkTally) {
	t.Helper()
	for _, colleague := range health.Colleagues {
		if ids.UUID(colleague.UserId) != user {
			continue
		}
		for _, tally := range colleague.Tallies {
			if tally.Kind == kind {
				return colleague, tally
			}
		}
		t.Fatalf("%s has no %s tally", user, kind)
	}
	t.Fatalf("%s is not in the report", user)
	return crmcontracts.AutomaticWorkColleague{}, crmcontracts.AutomaticWorkTally{}
}

func TestAutomaticWorkCountsEachOutcomeOncePerColleagueAndKind(t *testing.T) {
	f := newWorkFixture(t)
	e := f.e
	moveTo := func(user ids.UUID) activities.UpdateActivityInput {
		return activities.UpdateActivityInput{AssigneeID: new(ids.From[ids.UserKind](user))}
	}
	// Ten commitment tasks for Rep1: two handed to Rep2, one archived undone,
	// three done, four open.
	var tasks []ids.ActivityID
	for range 10 {
		tasks = append(tasks, f.promiseTask(t, e.Rep1))
	}
	f.update(t, tasks[0], moveTo(e.Rep2))
	f.update(t, tasks[1], moveTo(e.Rep2))
	// Handed back later, it was still not Rep1's when it arrived.
	f.update(t, tasks[1], moveTo(e.Rep1))
	if _, err := e.Activities.ArchiveActivity(e.Admin(), tasks[2], nil); err != nil {
		t.Fatal(err)
	}
	for _, done := range tasks[3:6] {
		f.update(t, done, activities.UpdateActivityInput{IsDone: new(true)})
	}
	// Done, reopened and done again is still one completed task.
	f.update(t, tasks[3], activities.UpdateActivityInput{IsDone: new(false)})
	f.update(t, tasks[3], activities.UpdateActivityInput{IsDone: new(true)})

	// A small sample: two reminders for Rep2, one handed back to Rep1.
	f.update(t, f.replyReminder(t, e.Rep2), moveTo(e.Rep1))
	f.replyReminder(t, e.Rep2)

	// Rep3 is asked three times and rejects one: the right colleague refusing the
	// change, which is no routing failure.
	asked := []ids.ApprovalID{f.approvalRequest(t, e.Rep3), f.approvalRequest(t, e.Rep3), f.approvalRequest(t, e.Rep3)}
	if _, err := f.approvals.Decide(f.seat(e.Rep3), asked[0], false, new("not this quarter")); err != nil {
		t.Fatal(err)
	}
	if _, err := f.approvals.Decide(f.seat(e.Rep3), asked[1], true, nil); err != nil {
		t.Fatal(err)
	}

	// Work a colleague made or asked for is not automatic work.
	manual := ids.From[ids.UserKind](e.Rep1)
	if _, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{Kind: "task", Subject: new("By hand"), AssigneeID: &manual}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.approvals.Stage(e.AgentCtx(), approvals.StageInput{
		Kind: string(workflow.ActionEmitFlowEvent), ProposedChange: json.RawMessage(`{"note":"from my agent"}`),
		DiffHash: "agent-" + ids.NewV7().String(), Summary: "my agent asks", TargetType: "contact",
		TargetID: e.SeedContact(t, "Agent Target", nil),
	}); err != nil {
		t.Fatal(err)
	}

	health := f.report(t, f.dbNow(t), 7)

	_, promises := tallyOf(t, health, e.Rep1, crmcontracts.AutomaticWorkKindNamePromiseTask)
	want := crmcontracts.AutomaticWorkCounts{Created: 10, NotMine: 2, Dismissed: 1, Completed: 3, Open: 4}
	if promises.Counts != want {
		t.Errorf("Rep1's promise tasks = %+v, want %+v", promises.Counts, want)
	}
	if promises.NotMineShare == nil || *promises.NotMineShare != 0.2 || promises.SmallSample || !promises.AboveThreshold {
		t.Errorf("Rep1's share = %v small=%v above=%v, want 0.2 on a sample large enough to flag",
			promises.NotMineShare, promises.SmallSample, promises.AboveThreshold)
	}
	if len(promises.ByDay) != 1 || promises.ByDay[0].Counts != want {
		t.Errorf("Rep1's days = %+v, want the one creation day holding every count", promises.ByDay)
	}

	// The two tasks handed to Rep2 count for the one they were made for.
	_, handed := tallyOf(t, health, e.Rep2, crmcontracts.AutomaticWorkKindNamePromiseTask)
	if handed.Counts.Created != 0 {
		t.Errorf("Rep2 is counted %d promise tasks that were made for Rep1", handed.Counts.Created)
	}
	_, reminders := tallyOf(t, health, e.Rep2, crmcontracts.AutomaticWorkKindNameReplyReminder)
	if reminders.Counts != (crmcontracts.AutomaticWorkCounts{Created: 2, NotMine: 1, Open: 1}) {
		t.Errorf("Rep2's reminders = %+v", reminders.Counts)
	}
	if reminders.NotMineShare == nil || *reminders.NotMineShare != 0.5 || !reminders.SmallSample || reminders.AboveThreshold {
		t.Errorf("a share on two items is shown and never flagged; got share=%v small=%v above=%v",
			reminders.NotMineShare, reminders.SmallSample, reminders.AboveThreshold)
	}

	_, decided := tallyOf(t, health, e.Rep3, crmcontracts.AutomaticWorkKindNameApprovalRequest)
	if decided.Counts != (crmcontracts.AutomaticWorkCounts{Created: 3, Rejected: 1, Completed: 1, Open: 1}) {
		t.Errorf("Rep3's requests = %+v", decided.Counts)
	}
	if decided.NotMineShare != nil {
		t.Errorf("an approval has no Not mine, so its share must be null, got %v", *decided.NotMineShare)
	}

	// The proposal Rep1's own agent staged is Rep1's work, not automation's.
	if _, own := tallyOf(t, health, e.Rep1, crmcontracts.AutomaticWorkKindNameApprovalRequest); own.Counts.Created != 0 {
		t.Errorf("Rep1 is counted %d approval requests, want none: their agent's proposal is their own", own.Counts.Created)
	}

	// A colleague with no automatic work reads as zero.
	_, none := tallyOf(t, health, e.AdminUser, crmcontracts.AutomaticWorkKindNamePromiseTask)
	if none.Counts != (crmcontracts.AutomaticWorkCounts{}) || none.NotMineShare != nil || len(none.ByDay) != 0 {
		t.Errorf("a colleague with nothing reads %+v", none)
	}
}

// An outcome counts on its item's creation day, however late it is read. The
// window drops the item once that day leaves it.
func TestAutomaticWorkCountsALateOutcomeOnTheCreationDay(t *testing.T) {
	f := newWorkFixture(t)
	task := f.promiseTask(t, f.e.Rep1)
	f.update(t, task, activities.UpdateActivityInput{IsDone: new(true)})
	created := f.dbNow(t)

	later := f.report(t, created.Add(72*time.Hour), 7)
	_, tally := tallyOf(t, later, f.e.Rep1, crmcontracts.AutomaticWorkKindNamePromiseTask)
	if tally.Counts.Completed != 1 || len(tally.ByDay) != 1 {
		t.Fatalf("read three days later: %+v", tally)
	}
	if day := tally.ByDay[0].Date.Format(time.DateOnly); day != created.Format(time.DateOnly) {
		t.Errorf("the outcome is counted on %s, want the creation day %s", day, created.Format(time.DateOnly))
	}

	past := f.report(t, created.Add(72*time.Hour), 2)
	if _, gone := tallyOf(t, past, f.e.Rep1, crmcontracts.AutomaticWorkKindNamePromiseTask); gone.Counts.Created != 0 {
		t.Errorf("a two-day window three days on still counts the item: %+v", gone.Counts)
	}
}

// A seat that was handed work and has since been deactivated still shows,
// under its own name and status.
func TestAutomaticWorkKeepsADeactivatedColleaguesItems(t *testing.T) {
	f := newWorkFixture(t)
	f.promiseTask(t, f.e.Rep3)
	if _, err := integration.OwnerConn(t).Exec(context.Background(),
		`UPDATE app_user SET status = 'deactivated' WHERE id = $1`, f.e.Rep3); err != nil {
		t.Fatal(err)
	}
	colleague, tally := tallyOf(t, f.report(t, f.dbNow(t), 7), f.e.Rep3, crmcontracts.AutomaticWorkKindNamePromiseTask)
	if colleague.Status != crmcontracts.AutomaticWorkColleagueStatusDeactivated || tally.Counts.Created != 1 {
		t.Errorf("a deactivated seat handed work reads %s with %d items", colleague.Status, tally.Counts.Created)
	}
}

func TestAutomaticWorkCountsAWaitingMessageMarkedNotMineOnce(t *testing.T) {
	f := newWorkFixture(t)
	direction := "inbound"
	message, _, err := f.e.Activities.LogActivity(f.e.Admin(), activities.LogActivityInput{
		Kind: "email", Subject: new("Can you confirm?"), Direction: &direction, Source: "manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	id := ids.From[ids.ActivityKind](ids.UUID(message.Id))
	reader := f.seat(f.e.Rep2)
	for _, step := range []func(context.Context, ids.ActivityID) error{
		f.e.Activities.SetMessageNotMine, f.e.Activities.ClearMessageDisposition, f.e.Activities.SetMessageNotMine,
	} {
		if err := step(reader, id); err != nil {
			t.Fatal(err)
		}
	}
	colleague, _ := tallyOf(t, f.report(t, f.dbNow(t), 7), f.e.Rep2, crmcontracts.AutomaticWorkKindNamePromiseTask)
	if colleague.WaitingNotMine != 1 {
		t.Errorf("waiting_not_mine = %d, want the one message marked twice", colleague.WaitingNotMine)
	}
}

func TestAutomaticWorkHealthIsForAnAdminOnly(t *testing.T) {
	f := newWorkFixture(t)
	h := automaticWorkHandlers{pool: f.e.Pool, now: func() time.Time { return f.dbNow(t) }}
	call := func(ctx context.Context, days *int) int {
		w := httptest.NewRecorder()
		r := httptest.NewRequestWithContext(ctx, http.MethodGet, "/v1/admin/automatic-work-health", nil)
		h.GetAutomaticWorkHealth(w, r, crmcontracts.GetAutomaticWorkHealthParams{Days: days})
		return w.Code
	}
	rep := f.e.As(f.e.Rep1, []ids.UUID{f.e.Team1}, principal.Permissions{
		Objects: map[string]principal.ObjectGrant{"activity": {Read: true}}, RowScope: principal.RowScopeAll,
	})
	if code := call(rep, nil); code != http.StatusForbidden {
		t.Errorf("a rep without job_health read got %d, want 403", code)
	}
	if code := call(f.e.AgentCtx(), nil); code != http.StatusForbidden {
		t.Errorf("an agent got %d, want 403", code)
	}
	if code := call(f.e.Admin(), new(30)); code != http.StatusOK {
		t.Errorf("an admin got %d, want 200", code)
	}
	if code := call(f.e.Admin(), new(31)); code != http.StatusUnprocessableEntity {
		t.Errorf("a 31-day window got %d, want 422", code)
	}
}

// A record's creator is its earliest create row, the one its source badge
// names. A later machine create row over a colleague's own task is a replay,
// not automatic work.
func TestAutomaticWorkLeavesOutATaskAColleagueCreatedFirst(t *testing.T) {
	f := newWorkFixture(t)
	task := f.promiseTask(t, f.e.Rep1)
	created := f.dbNow(t)
	f.e.WsExec(t, `
		INSERT INTO audit_log (id, actor_type, actor_id, action, entity_type, entity_id, occurred_at)
		VALUES ($1, 'human', $2, 'create', 'activity', $3, $4)`,
		ids.NewV7(), "human:"+f.e.Rep1.String(), task.UUID, created.Add(-time.Hour))

	health := f.report(t, created, 7)
	for _, colleague := range health.Colleagues {
		for _, tally := range colleague.Tallies {
			if tally.Kind == crmcontracts.AutomaticWorkKindNamePromiseTask && tally.Counts.Created != 0 {
				t.Errorf("a task a colleague created first counts as automatic work: %+v", tally.Counts)
			}
		}
	}
}

// A machine create replayed later is still one automatic item.
func TestAutomaticWorkCountsAReplayedMachineCreateOnce(t *testing.T) {
	f := newWorkFixture(t)
	task := f.promiseTask(t, f.e.Rep1)
	created := f.dbNow(t)
	f.e.WsExec(t, `
		INSERT INTO audit_log (actor_type, actor_id, action, entity_type, entity_id, occurred_at)
		SELECT actor_type, actor_id, 'create', entity_type, entity_id, occurred_at + interval '1 hour'
		  FROM audit_log WHERE entity_type = 'activity' AND entity_id = $1 AND action = 'create'`, task.UUID)

	_, tally := tallyOf(t, f.report(t, created.Add(2*time.Hour), 7), f.e.Rep1, crmcontracts.AutomaticWorkKindNamePromiseTask)
	if tally.Counts.Created != 1 {
		t.Errorf("a task whose machine create was replayed counts %d times, want once", tally.Counts.Created)
	}
}

func TestAutomaticWorkWindowStartsOnTheRequestedDayAcrossAMidnightDSTChange(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.Admin()
	zone := "America/Santiago"
	if _, err := identity.NewInstallationSettings(e.DB(), NewSettingsStore(e.Pool), StoredConfig(e.Pool)).UpdateInstallation(ctx, identity.InstallationPatch{Timezone: &zone}); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 6, 15, 0, 0, 0, time.UTC)
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		_, from, last, err := automaticWorkWindow(ctx, tx, now, 1)
		if err != nil {
			return err
		}
		if from.Format(time.DateOnly) != "2026-09-06" || !from.Equal(last) || from.Hour() != 1 {
			t.Fatalf("window starts %v, last day %v; want first valid instant on September 6", from, last)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
