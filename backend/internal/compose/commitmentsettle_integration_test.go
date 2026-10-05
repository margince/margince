// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// keptCommitment is a colleague's commitment in a meeting with one customer:
// a task for the colleague, and a claim on the customer pointing at it.
type keptCommitment struct {
	*transcriptEnv
	task, claim, customer ids.UUID
	settle                *CommitmentSettleTrigger
}

func seedKeptCommitment(t *testing.T) keptCommitment {
	t.Helper()
	e := setupTranscript(t)
	e.WsExec(t, `UPDATE app_user SET display_name = 'Priya Raman' WHERE id = $1`, e.Rep2)
	// Stored roles, because the settle asks the authority the seat really
	// holds rather than a permission set bound onto a test context.
	e.GrantRole(t, e.Rep2, "rep")
	e.GrantRole(t, e.Rep3, "rep")
	e.GrantRole(t, e.AdminUser, "admin")
	ines := linkInes(t, e)
	e.WsExec(t, `UPDATE contact SET owner_id = $2 WHERE id = $1`, ines, e.Rep2)
	e.read(t, cannedBrain{reply: ownedReply(t, "Priya Raman", 0.9)})
	task, err := ids.Parse(e.wsString(t, `SELECT id::text FROM activity WHERE kind = 'task'`))
	if err != nil {
		t.Fatalf("want the colleague's task written: %v", err)
	}
	claim, err := ids.Parse(e.wsString(t, `SELECT id::text FROM conversation_claim WHERE task_activity_id = $1`, task))
	if err != nil {
		t.Fatalf("want the claim on the customer pointing at the task: %v", err)
	}
	return keptCommitment{
		transcriptEnv: e, task: task, claim: claim, customer: ines,
		settle: NewCommitmentSettleTrigger(e.Pool, slog.Default()),
	}
}

// newest is the most recent outbox envelope of a type matching where, as the
// subscriber would have decoded it off the bus.
func (k keptCommitment) newest(t *testing.T, eventType, where string, arg any) events.Envelope {
	t.Helper()
	raw := k.wsString(t, `SELECT envelope::text FROM event_outbox
		WHERE envelope->>'type' = $1 AND `+where+` ORDER BY seq DESC LIMIT 1`, eventType, arg)
	var env events.Envelope
	if err := json.Unmarshal([]byte(raw), &env); err != nil {
		t.Fatalf("decoding the outbox envelope: %v", err)
	}
	return env
}

// setDone ticks or unticks the task as the colleague holding it.
func (k keptCommitment) setDone(t *testing.T, done bool) {
	t.Helper()
	priya := k.As(k.Rep2, []ids.UUID{k.Team1}, transcriptPerms)
	task, err := k.Activities.GetActivity(priya, ids.From[ids.ActivityKind](k.task), storekit.LiveOnly)
	if err != nil {
		t.Fatalf("reading the task: %v", err)
	}
	version := int64(*task.Version)
	if _, err := k.Activities.UpdateActivity(priya, ids.From[ids.ActivityKind](k.task),
		activities.UpdateActivityInput{IsDone: &done, IfVersion: &version}); err != nil {
		t.Fatalf("setting the task done=%v: %v", done, err)
	}
}

func (k keptCommitment) deliver(t *testing.T, env events.Envelope) {
	t.Helper()
	if err := k.settle.HandleEvent(context.Background(), env); err != nil {
		t.Fatalf("handling %s: %v", env.Type, err)
	}
}

func (k keptCommitment) taskDone(t *testing.T) string {
	t.Helper()
	return k.wsString(t, `SELECT is_done::text FROM activity WHERE id = $1`, k.task)
}

func (k keptCommitment) claimStatus(t *testing.T) string {
	t.Helper()
	return k.wsString(t, `SELECT status FROM conversation_claim WHERE id = $1`, k.claim)
}

// Ticking the task settles the commitment it stands for, and delivering the
// same event again, or the claim's own change that follows, changes nothing.
func TestTickingACommitmentsTaskSettlesTheCommitment(t *testing.T) {
	k := seedKeptCommitment(t)
	k.setDone(t, true)
	ticked := k.newest(t, eventActivityUpdated, `envelope->'entity'->>'id' = $2`, k.task.String())
	k.deliver(t, ticked)
	if got := k.claimStatus(t); got != claimStatusDone {
		t.Fatalf("the commitment reads %q after its task was ticked, want done", got)
	}
	k.deliver(t, ticked)
	k.deliver(t, k.newest(t, eventClaimChanged, `envelope->'payload'->>'claim_id' = $2`, k.claim.String()))
	if n := k.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'contact' AND after->>'claim_status' = 'done'`); n != 1 {
		t.Errorf("the commitment was settled %d times, want once", n)
	}
}

// Settling the commitment as kept completes its task.
func TestSettlingACommitmentAsKeptCompletesItsTask(t *testing.T) {
	k := seedKeptCommitment(t)
	if err := k.Contacts.SettleConversationClaim(k.Admin(), k.claim, claimStatusDone); err != nil {
		t.Fatalf("settling the commitment: %v", err)
	}
	k.deliver(t, k.newest(t, eventClaimChanged, `envelope->'payload'->>'claim_id' = $2`, k.claim.String()))
	if got := k.taskDone(t); got != "true" {
		t.Errorf("the task reads done=%s after its commitment was kept, want true", got)
	}
}

// A commitment a reader dismissed as never made leaves its task alone, and a
// task reopened after it was kept leaves its commitment kept.
func TestDismissingOrReopeningSettlesNothingElse(t *testing.T) {
	k := seedKeptCommitment(t)
	if err := k.Contacts.SettleConversationClaim(k.Admin(), k.claim, "dismissed"); err != nil {
		t.Fatalf("dismissing the commitment: %v", err)
	}
	k.deliver(t, k.newest(t, eventClaimChanged, `envelope->'payload'->>'claim_id' = $2`, k.claim.String()))
	if got := k.taskDone(t); got != "false" {
		t.Errorf("dismissing the commitment completed its task (done=%s)", got)
	}

	kept := seedKeptCommitment(t)
	kept.setDone(t, true)
	kept.deliver(t, kept.newest(t, eventActivityUpdated, `envelope->'entity'->>'id' = $2`, kept.task.String()))
	kept.setDone(t, false)
	kept.deliver(t, kept.newest(t, eventActivityUpdated, `envelope->'entity'->>'id' = $2`, kept.task.String()))
	if got := kept.claimStatus(t); got != claimStatusDone {
		t.Errorf("reopening the task moved its kept commitment to %q", got)
	}
}

// Settling a commitment does not complete a task whoever settles it could
// not change themselves: someone who may update contacts but no activity keeps
// their hands off the colleague's task.
func TestSettlingACommitmentLeavesATaskTheSettlerMayNotChange(t *testing.T) {
	k := seedKeptCommitment(t)
	k.WsExec(t, `DELETE FROM role_assignment WHERE user_id = $1`, k.Rep3)
	key := "claims-only-" + k.Rep3.String()
	k.WsExec(t, `INSERT INTO role (key, name, permissions) VALUES ($1, 'Claims only', $2::jsonb)`,
		key, `{"objects":{"contact":{"read":true,"update":true}},"row_scope":"all"}`)
	k.WsExec(t, `INSERT INTO role_assignment (role_id, user_id) SELECT r.id, $1 FROM role r WHERE r.key = $2`,
		k.Rep3, key)
	settler := k.As(k.Rep3, []ids.UUID{k.Team2}, principal.Permissions{
		RoleKeys: []string{key}, RowScope: principal.RowScopeAll,
		Objects: map[string]principal.ObjectGrant{"contact": {Read: true, Update: true}},
	})
	if err := k.Contacts.SettleConversationClaim(settler, k.claim, claimStatusDone); err != nil {
		t.Fatalf("settling the commitment: %v", err)
	}
	k.deliver(t, k.newest(t, eventClaimChanged, `envelope->'payload'->>'claim_id' = $2`, k.claim.String()))
	if got := k.taskDone(t); got != "false" {
		t.Errorf("someone with no activity grant completed a task by settling its commitment")
	}
}

// Ticking a task does not settle a commitment on a contact whoever ticks
// it may not update.
func TestTickingATaskLeavesACommitmentOnAContactTheyMayNotUpdate(t *testing.T) {
	k := seedKeptCommitment(t)
	k.WsExec(t, `UPDATE contact SET owner_id = $2 WHERE id = $1`, k.customer, k.Rep3)
	k.setDone(t, true)
	k.deliver(t, k.newest(t, eventActivityUpdated, `envelope->'entity'->>'id' = $2`, k.task.String()))
	if got := k.claimStatus(t); got != "open" {
		t.Errorf("ticking a task settled a commitment on a contact the rep may not update (status %q)", got)
	}
}
