// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The import job acts as whoever queued the attempt, in that attempt's trace.
// It acts under the authority that person or agent holds when the job runs.

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// queuedRun is a run whose attempt was queued by actor, as the store records it.
func queuedRun(actor principal.Principal, attempt int) migration.Run {
	stored := migration.StoredActorOf(actor)
	correlation := ids.NewV7()
	run := migration.Run{ID: migration.RunID(ids.NewV7())}
	run.JobAttempt, run.Actor, run.AttemptCorrelationID = attempt, &stored, &correlation
	return run
}

// agentOf is the principal the harness passport authenticates as today.
func agentOf(t *testing.T, e *Env) principal.Principal {
	t.Helper()
	live, err := identity.NewService(e.Pool).AuthenticateAgentByID(e.wsCtx(), ids.From[ids.PassportKind](e.AgentPassport))
	if err != nil {
		t.Fatalf("authenticating the harness passport: %v", err)
	}
	return live.Principal()
}

func (e *Env) wsCtx() context.Context { return principal.WithWorkspaceID(context.Background(), e.WS) }

func humanOf(user ids.UUID) principal.Principal {
	return principal.Principal{Type: principal.PrincipalHuman, ID: "human:" + user.String(), UserID: user}
}

func TestAnAgentApprovedRunWritesRowsAsTheAgent(t *testing.T) {
	e := Setup(t)
	e.GrantRole(t, e.Rep1, "rep")
	agent := agentOf(t, e)

	ctx, err := compose.ActAsImportRunForTest(e.wsCtx(), e.Pool, queuedRun(agent, 1))
	if err != nil {
		t.Fatalf("acting as the agent that approved the run: %v", err)
	}
	acting, ok := principal.Actor(ctx)
	if !ok {
		t.Fatal("the rebuilt context carries no actor")
	}
	if !acting.Scopes.Has(principal.ScopeWrite) || len(acting.Scopes) != 1 {
		t.Errorf("the agent acts with scopes %v, want write and nothing more", acting.Scopes)
	}
	if acting.OnBehalfOf != e.Rep1 || acting.PassportID != e.AgentPassport {
		t.Errorf("the agent acts for %s under passport %s, want %s under %s",
			acting.OnBehalfOf, acting.PassportID, e.Rep1, e.AgentPassport)
	}

	created, err := e.Contacts.CreateContact(ctx, contacts.CreateContactInput{FullName: "Imported by a tool", Source: "import"})
	if err != nil {
		t.Fatalf("writing a contact as the rebuilt agent: %v", err)
	}
	var capturedBy string
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT captured_by FROM contact WHERE id = $1`, created.Id).Scan(&capturedBy)
	}); err != nil {
		t.Fatalf("reading the contact's captured_by: %v", err)
	}
	if capturedBy != agent.ID {
		t.Errorf("the imported contact is captured by %q, want the approving agent %q", capturedBy, agent.ID)
	}
}

func TestARevokedPassportFailsTheRun(t *testing.T) {
	e := Setup(t)
	e.GrantRole(t, e.Rep1, "rep")
	run := queuedRun(agentOf(t, e), 1)
	revokePassport(t, e, e.AgentPassport)

	_, err := compose.ActAsImportRunForTest(e.wsCtx(), e.Pool, run)
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("acting under a revoked passport answered %v, want a permission refusal", err)
	}
}

func TestASuspendedApproverFailsTheRunWithPermission(t *testing.T) {
	for name, withdraw := range map[string]string{
		"deactivated": `UPDATE app_user SET status = 'deactivated' WHERE id = $1`,
		"read seat":   `UPDATE app_user SET seat_type = 'read' WHERE id = $1`,
	} {
		t.Run(name, func(t *testing.T) {
			e := Setup(t)
			e.GrantRole(t, e.Rep1, "rep")
			runs := []migration.Run{queuedRun(humanOf(e.Rep1), 1), queuedRun(agentOf(t, e), 1)}
			for _, run := range runs {
				if _, err := compose.ActAsImportRunForTest(e.wsCtx(), e.Pool, run); err != nil {
					t.Fatalf("the %s approver is refused before their seat changed: %v", run.Actor.Type, err)
				}
			}
			if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
				_, err := tx.Exec(context.Background(), withdraw, e.Rep1)
				return err
			}); err != nil {
				t.Fatalf("withdrawing the approver's seat: %v", err)
			}

			for _, run := range runs {
				_, err := compose.ActAsImportRunForTest(e.wsCtx(), e.Pool, run)
				if !errors.Is(err, apperrors.ErrPermissionDenied) {
					t.Errorf("acting as the %s %s approver answered %v, want a permission refusal", name, run.Actor.Type, err)
				}
			}
		})
	}
}

func TestEachAttemptCarriesItsOwnCorrelation(t *testing.T) {
	e := Setup(t)
	e.GrantRole(t, e.Rep1, "rep")
	first, second := queuedRun(humanOf(e.Rep1), 1), queuedRun(humanOf(e.Rep1), 2)

	for _, run := range []migration.Run{first, second, first} {
		ctx, err := compose.ActAsImportRunForTest(e.wsCtx(), e.Pool, run)
		if err != nil {
			t.Fatalf("acting as attempt %d: %v", run.JobAttempt, err)
		}
		got, ok := principal.CorrelationID(ctx)
		if !ok || got != *run.AttemptCorrelationID {
			t.Errorf("attempt %d runs in trace %v, want its own %v", run.JobAttempt, got, *run.AttemptCorrelationID)
		}
	}
}

func TestAnAgentActsWithNoVerbItsPassportLacks(t *testing.T) {
	e := Setup(t)
	e.GrantRole(t, e.Rep1, "rep")
	run := queuedRun(agentOf(t, e), 1)
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `UPDATE passport SET scopes = ARRAY['read'] WHERE id = $1`, e.AgentPassport)
		return err
	}); err != nil {
		t.Fatalf("narrowing the passport to read: %v", err)
	}

	ctx, err := compose.ActAsImportRunForTest(e.wsCtx(), e.Pool, run)
	if err != nil {
		t.Fatalf("acting as a read-only agent: %v", err)
	}
	acting, ok := principal.Actor(ctx)
	if !ok || acting.Scopes.Has(principal.ScopeWrite) {
		t.Errorf("an agent whose passport holds only read acts with scopes %v, want no write", acting.Scopes)
	}
}
