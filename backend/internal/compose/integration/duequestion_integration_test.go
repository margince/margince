// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/duequestion"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

var ownerPerms = principal.Permissions{
	RoleKeys: []string{"rep"},
	Objects:  map[string]principal.ObjectGrant{"activity": {Create: true, Read: true, Update: true}, "installation_settings": {Read: true}},
	RowScope: principal.RowScopeAll,
}

func fileExtensionTask(t *testing.T, e *Env, key string, assignee ids.UUID, due time.Time) error {
	t.Helper()
	ctx := principal.WithActor(connectorCtx(e, e.Rep1), principal.Principal{
		Type: principal.PrincipalConnector, ID: "connector:ingress", UserID: e.Rep1, OnBehalfOf: e.Rep1,
		Permissions: principal.Permissions{Objects: map[string]principal.ObjectGrant{"activity": {Create: true, Read: true}}, RowScope: principal.RowScopeAll},
	})
	sink := capture.NewSink(e.DB()).WithClock(func() time.Time { return clocktest.Now(t) }).
		WithTaskAssigneeCheck(activities.CheckNewTaskAssignee).
		WithDueAsker(duequestion.Asker{Approvals: compose.NewApprovalsService(e.DB())})
	_, err := sink.Upsert(ctx, connector.NormalizedRecord{
		EntityType: datasource.EntityActivity,
		NaturalKey: connector.NaturalKey{SourceSystem: "unit", SourceID: key},
		Fields:     capture.ActivityFields{Kind: "task", Subject: "Send the quote", OccurredAt: clocktest.Now(t), AssigneeID: assignee, DueAt: due},
		Source:     "unit", CapturedBy: "connector:ingress",
	})
	return err
}

func dueQuestions(t *testing.T, e *Env) int {
	t.Helper()
	return e.WsCount(t, `SELECT count(*) FROM approval WHERE kind = 'task_due_date'`)
}

// askedFarDate files a task with a far due date for the first rep and returns
// the question that waits for them, with its staged payload.
func askedFarDate(t *testing.T, e *Env, key string, due time.Time) (ids.ApprovalID, map[string]any) {
	t.Helper()
	if err := fileExtensionTask(t, e, key, e.Rep1, due); err != nil {
		t.Fatalf("filing the task: %v", err)
	}
	var id ids.UUID
	var raw []byte
	if err := OwnerConn(t).QueryRow(context.Background(), `SELECT id, proposed_change FROM approval WHERE kind = 'task_due_date'`).Scan(&id, &raw); err != nil {
		t.Fatalf("reading the question: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	return ids.From[ids.ApprovalKind](id), payload
}

func answerAs(e *Env, user ids.UUID) context.Context {
	return e.As(user, []ids.UUID{e.Team1}, ownerPerms)
}

func decideEdited(e *Env, user ids.UUID, id ids.ApprovalID, payload map[string]any, edit map[string]any) error {
	edited := map[string]any{}
	maps.Copy(edited, payload)
	maps.Copy(edited, edit)
	raw, err := json.Marshal(edited)
	if err != nil {
		return err
	}
	_, err = compose.NewApprovalsServiceWithEffects(e.Pool).DecideEdited(answerAs(e, user), id, raw)
	return err
}

func dueOf(t *testing.T, key string) (set bool, year string) {
	t.Helper()
	if err := OwnerConn(t).QueryRow(context.Background(), `SELECT due_at IS NOT NULL, coalesce(to_char(due_at AT TIME ZONE 'UTC', 'YYYY'), '') FROM activity WHERE source_id = $1`, key).Scan(&set, &year); err != nil {
		t.Fatal(err)
	}
	return set, year
}

func TestAFarDueDateOnACapturedTaskIsAskedOnceAndKeptByTheOwner(t *testing.T) {
	t.Parallel()
	e := Setup(t)
	far := clocktest.Now(t).AddDate(15, 0, 0)
	id, _ := askedFarDate(t, e, "far", far)
	if err := fileExtensionTask(t, e, "far", e.Rep1, far); err != nil {
		t.Fatal(err)
	}
	if set, _ := dueOf(t, "far"); set || dueQuestions(t, e) != 1 {
		t.Fatalf("a far date left a dated task or %d questions, want an undated task and 1", dueQuestions(t, e))
	}
	if _, err := compose.NewApprovalsServiceWithEffects(e.Pool).Decide(answerAs(e, e.Rep1), id, true, nil); err != nil {
		t.Fatalf("keeping the date: %v", err)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE action = 'update' AND entity_type = 'activity' AND actor_id = 'human:' || $1`, e.Rep1.String()); n == 0 {
		t.Error("the answer did not write through the audited update path as the owner")
	}
	if _, year := dueOf(t, "far"); year != far.Format("2006") {
		t.Errorf("the kept date has year %q, want %s", year, far.Format("2006"))
	}
}

func TestTheOwnerMayClearOrReplaceTheDate(t *testing.T) {
	t.Parallel()
	e := Setup(t)
	id, payload := askedFarDate(t, e, "clear", clocktest.Now(t).AddDate(15, 0, 0))
	if err := decideEdited(e, e.Rep1, id, payload, map[string]any{"due_date": ""}); err != nil {
		t.Fatalf("clearing the date: %v", err)
	}
	if set, _ := dueOf(t, "clear"); set {
		t.Error("a cleared date left a due date on the task")
	}
	t.Run("replace", func(t *testing.T) { replaceTheDate(t, Setup(t)) })
}

func replaceTheDate(t *testing.T, e *Env) {
	id, payload := askedFarDate(t, e, "replace", clocktest.Now(t).AddDate(16, 0, 0))
	next := clocktest.Now(t).AddDate(0, 0, 10).Format(time.DateOnly)
	if err := decideEdited(e, e.Rep1, id, payload, map[string]any{"due_date": next}); err != nil {
		t.Fatalf("replacing the date: %v", err)
	}
	if set, _ := dueOf(t, "replace"); !set {
		t.Error("a valid replacement date was not written")
	}
}

func TestATamperedAnswerOrAnotherReaderIsRefused(t *testing.T) {
	t.Parallel()
	e := Setup(t)
	id, payload := askedFarDate(t, e, "tamper", clocktest.Now(t).AddDate(15, 0, 0))
	other := clocktest.Now(t).AddDate(20, 0, 0).Format(time.DateOnly)
	if err := decideEdited(e, e.Rep1, id, payload, map[string]any{"odd_due_date": other, "due_date": other}); err == nil {
		t.Error("a rewritten odd date and a new far date were accepted")
	}
	if err := decideEdited(e, e.Rep1, id, payload, map[string]any{"due_date": other}); err == nil {
		t.Error("another far date was accepted")
	}
	if _, err := compose.NewApprovalsServiceWithEffects(e.Pool).Decide(answerAs(e, e.Rep2), id, true, nil); err == nil {
		t.Error("a colleague who does not own the task answered its question")
	}
	if set, _ := dueOf(t, "tamper"); set {
		t.Error("a refused answer still wrote a date")
	}
}

func TestAnAnswerAfterTheTaskChangedWritesNothing(t *testing.T) {
	t.Parallel()
	for name, change := range map[string]struct {
		sql   string
		value func(t *testing.T, e *Env) any
	}{
		"dated by hand": {
			`UPDATE activity SET due_at = $1 WHERE source_id = 'stale'`,
			func(t *testing.T, _ *Env) any { return clocktest.Now(t).AddDate(0, 0, 3) },
		},
		"reassigned": {
			`UPDATE activity SET assignee_id = $1 WHERE source_id = 'stale'`,
			func(_ *testing.T, e *Env) any { return e.Rep2 },
		},
	} {
		t.Run(name, func(t *testing.T) {
			e := Setup(t)
			id, _ := askedFarDate(t, e, "stale", clocktest.Now(t).AddDate(15, 0, 0))
			e.WsExec(t, change.sql, change.value(t, e))
			_, err := compose.NewApprovalsServiceWithEffects(e.Pool).Decide(answerAs(e, e.Rep1), id, true, nil)
			if _, ok := errors.AsType[*approvals.ObsoleteError](err); !ok {
				t.Fatalf("answering a stale question gave %v, want an obsolete card", err)
			}
			if _, year := dueOf(t, "stale"); year == clocktest.Now(t).AddDate(15, 0, 0).Format("2006") {
				t.Error("the stale answer wrote the far date")
			}
		})
	}
}

func TestTheQuestionNamesTheDayInTheInstallationZone(t *testing.T) {
	t.Parallel()
	e := Setup(t)
	e.WsExec(t, `UPDATE setting SET value = '"America/Los_Angeles"'::jsonb WHERE key = 'installation.timezone'`)
	day := clocktest.Now(t).AddDate(15, 0, 0).UTC().Truncate(24 * time.Hour)
	_, payload := askedFarDate(t, e, "zone", day.Add(3*time.Hour))
	if want := day.AddDate(0, 0, -1).Format(time.DateOnly); payload["due_date"] != want {
		t.Errorf("the question names %v, want %s (the day it is in Los Angeles)", payload["due_date"], want)
	}
}

func TestADueDateInsideTheWindowOrWithNoOwnerAsksNobody(t *testing.T) {
	t.Parallel()
	e := Setup(t)
	if err := fileExtensionTask(t, e, "near", e.Rep1, clocktest.Now(t).AddDate(0, 0, 7)); err != nil {
		t.Fatal(err)
	}
	if err := fileExtensionTask(t, e, "unowned", ids.Nil, clocktest.Now(t).AddDate(15, 0, 0)); err != nil {
		t.Fatal(err)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM activity WHERE source_id = 'near' AND due_at IS NOT NULL`); n != 1 || dueQuestions(t, e) != 0 {
		t.Errorf("near task dated=%d, questions=%d; want 1 and 0", n, dueQuestions(t, e))
	}
}
