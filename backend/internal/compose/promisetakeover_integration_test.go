// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A colleague takes over a promise nobody was asked to keep. The claims here
// are filed through the claim writer. That leaves them as an unassignable
// promise is left: open on the contact, with no task.

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type takeOverFixture struct {
	e       *integration.Env
	h       promiseTakeOverHandlers
	contact ids.UUID
	claim   ids.UUID
	source  ids.UUID
	priya   context.Context
}

func setupTakeOver(t *testing.T) takeOverFixture {
	t.Helper()
	e := integration.Setup(t)
	e.WsExec(t, `UPDATE app_user SET display_name = 'Priya Raman' WHERE id = $1`, e.Rep2)
	contact := e.SeedContact(t, "Frau Adler", &e.Rep1)
	claim := ids.UUID(seedWorklistPromise(t, e, contact, "Send the signed contract by Friday").Id)
	source := ids.MustParse(e.WsScalar(t, `SELECT source_activity_id::text FROM conversation_claim WHERE id = $1`, claim))
	return takeOverFixture{
		e: e, h: newPromiseTakeOverHandlers(e.Pool), contact: contact, claim: claim, source: source,
		priya: e.As(e.Rep2, []ids.UUID{e.Team1}, integration.AdminPerms),
	}
}

func (f takeOverFixture) tasks(t *testing.T) int {
	t.Helper()
	return f.e.WsCount(t, `SELECT count(*) FROM activity WHERE kind = 'task' AND source_system = $1`, activities.CommitmentTaskSource)
}

func (f takeOverFixture) takeOverAudits(t *testing.T) int {
	t.Helper()
	return f.e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'contact' AND entity_id = $1
		AND after ? 'taken_by_user_id'`, f.contact)
}

func TestAColleagueTakesOverAPromiseNobodyWasAskedToKeep(t *testing.T) {
	f := setupTakeOver(t)
	out, err := f.h.TakeOver(f.priya, f.claim)
	if err != nil {
		t.Fatalf("taking over the promise: %v", err)
	}
	if n := f.tasks(t); n != 1 {
		t.Fatalf("taking over wrote %d commitment tasks, want 1", n)
	}
	task := ids.UUID(out.TaskActivityId)
	held := f.e.WsScalar(t, `SELECT assignee_id::text || ' ' || subject || ' ' || origin FROM activity WHERE id = $1`, task)
	if want := f.e.Rep2.String() + " Send the signed contract by Friday human"; held != want {
		t.Errorf("the task reads %q, want %q", held, want)
	}
	linked := f.e.WsCount(t, `SELECT count(*) FROM activity_link WHERE activity_id = $1 AND contact_id = $2`, task, f.contact)
	if linked != 1 {
		t.Errorf("the task is linked to the contact %d times, want once", linked)
	}
	if ids.UUID(out.TakenByUserId) != f.e.Rep2 || out.TakenByName == nil || *out.TakenByName != "Priya Raman" {
		t.Errorf("the answer names %v %v as the taker, want Priya Raman", out.TakenByUserId, out.TakenByName)
	}

	var claims []crmcontracts.ConversationClaim
	if err := database.WithWorkspaceTx(f.priya, f.e.Pool, func(tx pgx.Tx) error {
		var err error
		claims, err = f.e.Contacts.ClaimsForContact(f.priya, tx, ids.From[ids.ContactKind](f.contact), nil, 10)
		return err
	}); err != nil {
		t.Fatalf("reading the contact's claims: %v", err)
	}
	if len(claims) != 1 || claims[0].TakenByName == nil || *claims[0].TakenByName != "Priya Raman" ||
		claims[0].TakenAt == nil || claims[0].TaskActivityId == nil || ids.UUID(*claims[0].TaskActivityId) != task {
		t.Fatalf("the record does not show who took the promise over: %+v", claims)
	}

	if n := f.takeOverAudits(t); n != 1 {
		t.Errorf("%d audit rows record the take over, want 1", n)
	}
	events := f.e.WsCount(t, `SELECT count(*) FROM event_outbox e
		JOIN audit_log a ON a.id = (e.envelope->'trace'->>'audit_log_id')::uuid
		WHERE e.envelope->>'type' = 'conversation_claim.changed' AND a.after ? 'taken_by_user_id'`)
	if events != 1 {
		t.Errorf("%d changed events ride the take over's audit row, want 1", events)
	}
}

func TestAPromiseTakenOverTwiceIsStillOneTask(t *testing.T) {
	f := setupTakeOver(t)
	first, err := f.h.TakeOver(f.priya, f.claim)
	if err != nil {
		t.Fatalf("taking over the promise: %v", err)
	}
	again, err := f.h.TakeOver(f.priya, f.claim)
	if err != nil {
		t.Fatalf("pressing a second time: %v", err)
	}
	if again.TaskActivityId != first.TaskActivityId || !again.TakenAt.Equal(first.TakenAt) {
		t.Errorf("a second press answered %+v, want the first take over %+v", again, first)
	}

	colleague := f.e.As(f.e.Rep1, []ids.UUID{f.e.Team1}, integration.AdminPerms)
	_, err = f.h.TakeOver(colleague, f.claim)
	if !errors.Is(err, apperrors.ErrConflict) || !strings.Contains(err.Error(), "Priya Raman took this promise over") {
		t.Fatalf("a second colleague got %v, want a conflict naming Priya Raman", err)
	}
	if n := f.tasks(t); n != 1 {
		t.Errorf("three presses wrote %d tasks, want 1", n)
	}
	if n := f.takeOverAudits(t); n != 1 {
		t.Errorf("three presses wrote %d take over audit rows, want 1", n)
	}

	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/claims/"+f.claim.String()+"/take-over", nil).WithContext(colleague)
	f.h.TakeOverConversationClaim(response, request, crmcontracts.Id(f.claim))
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "Priya Raman") {
		t.Errorf("the second colleague's request answered %d %s, want 409 naming Priya Raman", response.Code, response.Body.String())
	}
}

func TestAPromiseWhoseSourceTheReaderCannotReadIsNotTheirsToTake(t *testing.T) {
	f := setupTakeOver(t)
	if _, err := f.e.Activities.SetAudience(f.e.Admin(), ids.From[ids.ActivityKind](f.source), activities.SetAudienceInput{
		Audience: "selected", Members: []activities.AudienceMember{{SubjectType: "user", SubjectID: f.e.Rep1}},
	}); err != nil {
		t.Fatalf("narrowing the source message: %v", err)
	}
	if _, err := f.h.TakeOver(f.priya, f.claim); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("a reader who may not read the source got %v, want not found", err)
	}
	if n := f.tasks(t); n != 0 {
		t.Errorf("a refused take over wrote %d tasks", n)
	}
	if got := f.e.WsScalar(t, `SELECT coalesce(taken_by_user_id::text, 'nobody') FROM conversation_claim WHERE id = $1`, f.claim); got != "nobody" {
		t.Errorf("a refused take over named %s as the taker", got)
	}
}

func TestAnAgentCannotTakeOverAPromise(t *testing.T) {
	f := setupTakeOver(t)
	agent := f.e.AgentFor(t, f.e.Rep2, []ids.UUID{f.e.Team1}, integration.AdminPerms)
	if _, err := f.h.TakeOver(agent, f.claim); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("an agent got %v, want permission denied", err)
	}
	if n := f.tasks(t); n != 0 {
		t.Errorf("an agent's take over wrote %d tasks", n)
	}
}

func TestAPromiseFromNarrowedMailBecomesATaskOnlyItsTakerReads(t *testing.T) {
	f := setupTakeOver(t)
	if _, err := f.e.Activities.SetAudience(f.e.Admin(), ids.From[ids.ActivityKind](f.source), activities.SetAudienceInput{
		Audience: "selected", Members: []activities.AudienceMember{
			{SubjectType: "user", SubjectID: f.e.Rep1}, {SubjectType: "user", SubjectID: f.e.Rep2},
		},
	}); err != nil {
		t.Fatalf("narrowing the source message: %v", err)
	}
	out, err := f.h.TakeOver(f.priya, f.claim)
	if err != nil {
		t.Fatalf("taking over the promise: %v", err)
	}
	task := ids.From[ids.ActivityKind](ids.UUID(out.TaskActivityId))
	if _, err := f.e.Activities.GetActivity(f.priya, task, storekit.LiveOnly); err != nil {
		t.Fatalf("the taker cannot read their own task: %v", err)
	}

	outsider := f.e.As(f.e.Rep3, []ids.UUID{f.e.Team2}, integration.AdminPerms)
	if _, err := f.e.Activities.GetActivityContent(outsider, task, storekit.LiveOnly); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("a colleague who may not read the mail got %v on the task, want not found", err)
	}
	// Rep1 may read the mail but did not take the promise: the task is the
	// taker's alone, as the task holds the promise's words.
	reader := f.e.As(f.e.Rep1, []ids.UUID{f.e.Team1}, integration.AdminPerms)
	kind := "task"
	for name, ctx := range map[string]context.Context{"outsider": outsider, "other reader": reader} {
		rows, _, err := f.e.Activities.ListActivities(ctx, activities.ListActivitiesInput{Kind: &kind})
		if err != nil {
			t.Fatalf("listing tasks as the %s: %v", name, err)
		}
		for _, row := range rows {
			if row.Subject != nil && strings.Contains(*row.Subject, "signed contract") {
				t.Errorf("the %s reads the promise in a task list: %q", name, *row.Subject)
			}
			if row.Body != nil && strings.Contains(*row.Body, "signed contract") {
				t.Errorf("the %s reads the promise in a task body: %q", name, *row.Body)
			}
		}
	}
}
