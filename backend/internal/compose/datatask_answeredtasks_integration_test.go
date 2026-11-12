// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// answeredTaskFixture is the admin's open task on a contact, and the admin's
// later mail to that contact, stored without the live rule running.
func answeredTaskFixture(t *testing.T, e *integration.Env, address string, answered bool) ids.UUID {
	t.Helper()
	contact := e.SeedContact(t, "Stored answer "+address, nil)
	e.WsExec(t, `INSERT INTO contact_email (contact_id,email,email_type,source,captured_by) VALUES ($1,$2,'work','manual','human:test')`, contact, address)
	task, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{Kind: "task", Subject: new("Send the file"), Source: "manual", AssigneeID: new(ids.From[ids.UserKind](e.AdminUser)), Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}}})
	if err != nil {
		t.Fatal(err)
	}
	if answered {
		direction, at := "outbound", task.CreatedAt.Add(time.Hour)
		if _, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{Kind: "email", Subject: new("The file"), Direction: &direction, OccurredAt: &at, EmailFrom: "admin@stored.test", EmailTo: []string{address}, CounterpartyOutboundAttested: true, Source: "manual"}); err != nil {
			t.Fatal(err)
		}
	}
	return ids.UUID(task.Id)
}

func TestTheAnsweredTasksTaskClosesStoredAnswersOncePageByPage(t *testing.T) {
	e := integration.Setup(t)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	answered := []ids.UUID{answeredTaskFixture(t, e, "one@stored.test", true), answeredTaskFixture(t, e, "two@stored.test", true), answeredTaskFixture(t, e, "three@stored.test", true)}
	unanswered := answeredTaskFixture(t, e, "none@stored.test", false)
	task := answeredTasksDataTask(e.Pool)
	if n, _, err := task.Steps[0].Estimate(ctx, e.Pool, 2); err != nil || n != 2 {
		t.Fatalf("estimate=%d, %v; want the cap of 2", n, err)
	}
	task.Steps[0].Limit = 1
	for range 2 {
		runCalendarDescriptionTask(ctx, t, e, task)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM activity WHERE id=ANY($1) AND is_done`, answered); got != len(answered) {
		t.Fatalf("closed %d of %d answered tasks", got, len(answered))
	}
	if got := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_id=ANY($1) AND evidence ? 'task_answered_by'`, answered); got != len(answered) {
		t.Fatalf("completion audits=%d across two runs, want one per task", got)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM audit_log a JOIN event_outbox o ON o.envelope->'trace'->>'audit_log_id'=a.id::text
 AND o.envelope->>'type'='activity.updated' AND o.envelope->'entity'->>'id'=a.entity_id::text
 WHERE a.entity_id=ANY($1) AND a.evidence ? 'task_answered_by'`, answered); got != len(answered) {
		t.Fatalf("completion events linked to their audit rows=%d across two runs, want one per task", got)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM event_outbox WHERE envelope->>'type'='activity.updated' AND envelope->'entity'->>'id'=ANY($1)`, idStrings(answered)); got != len(answered) {
		t.Fatalf("activity.updated events=%d across two runs, want one per task", got)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM activity WHERE id=$1 AND is_done`, unanswered); got != 0 {
		t.Fatal("a task with no later mail was closed")
	}
}

func idStrings(named []ids.UUID) []string {
	out := make([]string, len(named))
	for i, id := range named {
		out[i] = id.String()
	}
	return out
}

func TestTheAnsweredTasksTaskRollsBackWithItsProgress(t *testing.T) {
	e := integration.Setup(t)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	answered := answeredTaskFixture(t, e, "rollback@stored.test", true)
	task := answeredTasksDataTask(e.Pool)
	original := task.Steps[0].Batch
	task.Steps[0].Batch = func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
		out, err := original(ctx, tx, in)
		if err != nil {
			return out, err
		}
		return out, errors.New("transaction interrupted after domain writes")
	}
	runCalendarDescriptionTask(ctx, t, e, task)
	if got := e.WsCount(t, `SELECT count(*) FROM activity WHERE id=$1 AND is_done`, answered); got != 0 {
		t.Fatal("the completion survived a rolled-back batch")
	}
	if got := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_id=$1 AND evidence ? 'task_answered_by'`, answered); got != 0 {
		t.Fatalf("completion audits escaped the rollback: %d", got)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM event_outbox WHERE envelope->>'type'='activity.updated' AND envelope->'entity'->>'id'=$1`, answered.String()); got != 0 {
		t.Fatalf("completion events escaped the rollback: %d", got)
	}
}
