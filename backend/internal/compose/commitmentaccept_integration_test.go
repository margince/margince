// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// Accepting a commitment read out of mail asks the thread rule again, so a
// conversation that changed while the card waited is not written as a task
// the old answer would have allowed.

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const mailedPromise = "We will send the revised pricing over by Friday."

// mailedProposal is an unsure commitment a colleague made in a shared mail
// thread with a customer, staged for that colleague to accept.
type mailedProposal struct {
	*integration.Env
	customer, approval ids.UUID
}

func stageMailedProposal(t *testing.T) mailedProposal {
	t.Helper()
	e := integration.Setup(t)
	company := e.SeedCompany(t, "Acme", &e.Rep1)
	ines := e.SeedContact(t, "Ines Huber", &e.Rep1)
	e.WsExec(t, `INSERT INTO relationship (kind, contact_id, company_id, source, captured_by)
		VALUES ('employment', $1, $2, 'manual', 'human:x')`, ines, company)
	sent := time.Now().Add(-48 * time.Hour).UTC().Format(time.RFC3339Nano)
	message := integration.SeedIDRow(t, integration.OwnerConn(t), `INSERT INTO activity
		(id, kind, direction, subject, body, thread_key, occurred_at, created_at, source, captured_by)
		VALUES ($1, 'email', 'outbound', 'Pricing', 'Hi Ines, `+mailedPromise+`', 'thread-accept',
		        '`+sent+`', '`+sent+`', 'gmail', 'connector:gmail')`)
	integration.LinkActivity(t, integration.OwnerConn(t), message, "contact", ines)
	e.WsExec(t, `INSERT INTO activity_participant (activity_id, user_id, contact_id, role) VALUES ($1, $2, NULL, 'from')`, message, e.Rep2)
	e.WsExec(t, `INSERT INTO activity_participant (activity_id, user_id, contact_id, role) VALUES ($1, NULL, $2, 'to')`, message, ines)

	reply, err := json.Marshal(map[string]any{"events": []map[string]any{{
		"kind": "commitment_made", "message_id": message.String(), "summary": "Send the revised pricing",
		"quote": mailedPromise, "due_date": "", "confidence": 0.75,
	}}})
	if err != nil {
		t.Fatalf("building the model reply: %v", err)
	}
	pass := principal.SystemActing(principal.WithWorkspaceID(context.Background(), e.WS), signalScanActor)
	if _, err := NewSignalExtractor(e.Pool, cannedBrain{reply: string(reply)}, time.Now, slog.Default()).
		RunWorkspace(pass, ids.From[ids.WorkspaceKind](e.WS)); err != nil {
		t.Fatalf("reading the thread: %v", err)
	}
	approval, err := ids.Parse(e.WsScalar(t, `SELECT id::text FROM approval WHERE kind = 'commitment_task'`))
	if err != nil {
		t.Fatalf("want the unsure commitment proposed: %v", err)
	}
	return mailedProposal{Env: e, customer: ines, approval: approval}
}

// The account counts the commitment while it waits, for the product's own pass
// as for anyone who may read the whole account.
func TestAWaitingMailedCommitmentIsCountedOnItsAccount(t *testing.T) {
	m := stageMailedProposal(t)
	pass := principal.SystemActing(principal.WithWorkspaceID(context.Background(), m.WS), signalScanActor)
	company, err := ids.Parse(m.WsScalar(t, `SELECT company_id::text FROM relationship WHERE contact_id = $1`, m.customer))
	if err != nil {
		t.Fatalf("reading the account: %v", err)
	}
	var open int
	var complete bool
	if err := database.WithWorkspaceTx(pass, m.Pool, func(tx pgx.Tx) error {
		var err error
		open, complete, err = m.Contacts.CountAccountCommitments(pass, tx, company)
		return err
	}); err != nil || open != 1 || !complete {
		t.Errorf("the account counts %d commitment(s), complete=%v, err=%v; want the one waiting", open, complete, err)
	}
}

func (m mailedProposal) accept() error {
	colleague := m.As(m.Rep2, []ids.UUID{m.Team1}, transcriptPerms)
	_, err := approvalsServiceWithEffects(m.Pool).Decide(colleague, ids.From[ids.ApprovalKind](m.approval), true, nil)
	return err
}

// The colleague accepts their own commitment from a thread that is still
// shared, and it becomes their task.
func TestAColleagueAcceptsTheirMailedCommitment(t *testing.T) {
	m := stageMailedProposal(t)
	if err := m.accept(); err != nil {
		t.Fatalf("accepting: %v", err)
	}
	if got := m.WsScalar(t, `SELECT coalesce(assignee_id::text, '') FROM activity WHERE kind = 'task'`); got != m.Rep2.String() {
		t.Errorf("the task is held by %q, want the colleague who made the commitment", got)
	}
}

// A thread that reaches a second account while the card waited is no longer
// one the rule reads, and acceptance is refused rather than guessing who may
// read the task.
func TestAMailedCommitmentWhoseThreadChangedIsNotAccepted(t *testing.T) {
	m := stageMailedProposal(t)
	other := m.SeedCompany(t, "Beta", &m.Rep1)
	m.WsExec(t, `INSERT INTO relationship (kind, contact_id, company_id, source, captured_by)
		VALUES ('employment', $1, $2, 'manual', 'human:x')`, m.customer, other)
	if err := m.accept(); !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("accepting after the thread changed: err = %v, want a conflict", err)
	}
	if n := m.WsCount(t, `SELECT count(*) FROM activity WHERE kind = 'task'`); n != 0 {
		t.Errorf("a refused acceptance wrote %d task(s)", n)
	}
}

// A thread made private to another member while the card waited narrows the
// commitment to that member, so the colleague may not accept it.
func TestAMailedCommitmentMadePrivateSinceIsTheOwnersToAccept(t *testing.T) {
	m := stageMailedProposal(t)
	m.WsExec(t, `UPDATE contact SET visibility = 'owner', owner_id = $2 WHERE id = $1`, m.customer, m.Rep1)
	err := m.accept()
	if err == nil {
		t.Fatal("the colleague accepted a commitment from mail now private to another member")
	}
	if n := m.WsCount(t, `SELECT count(*) FROM activity WHERE kind = 'task'`); n != 0 {
		t.Errorf("a refused acceptance wrote %d task(s)", n)
	}
}
