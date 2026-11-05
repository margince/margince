// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	capturemod "github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// agentOfSeat is the context of an agent on the user's own passport.
func agentOfSeat(seat context.Context, t *testing.T) context.Context {
	t.Helper()
	agent, ok := principal.Actor(seat)
	if !ok {
		t.Fatal("no actor")
	}
	agent.Type, agent.ID = principal.PrincipalAgent, "agent:"+ids.NewV7().String()
	agent.PassportID, agent.OnBehalfOf = ids.NewV7(), agent.UserID
	return principal.WithActor(seat, agent)
}

// An agent changes its user's own mailbox settings on a real connection, then
// disconnects it. A passportless agent, which is automatic work, is refused.
func TestAnAgentSetsAndDisconnectsItsUsersMailbox(t *testing.T) {
	env := newCaptureEnv(t)
	seat := seatContext(env.e, env.e.Rep1)
	agent := agentOfSeat(seat, t)

	off := false
	if _, err := env.registry.SetSignatureEnrichment(agent, "gmail", &off); err != nil {
		t.Fatalf("setting the signature pass: %v", err)
	}
	if _, err := env.registry.SetMailPosture(agent, "gmail", capturemod.PostureClassified, false); err != nil {
		t.Fatalf("setting the posture: %v", err)
	}
	day := clocktest.Now(t).AddDate(0, -1, 0)
	if _, err := env.registry.SetHistoryBaseline(agent, "gmail", &day, "UTC"); err != nil {
		t.Fatalf("setting the history baseline: %v", err)
	}
	if _, err := env.registry.SetContextTag(agent, "gmail", nil); err != nil {
		t.Fatalf("clearing the filing word: %v", err)
	}
	views, err := env.registry.Connections(agent)
	if err != nil || len(views) == 0 {
		t.Fatalf("the agent listing its user's mailboxes = %d (%v), want the one", len(views), err)
	}

	automatic, _ := principal.Actor(agent)
	automatic.PassportID = ids.UUID{}
	if _, err := env.registry.SetSignatureEnrichment(principal.WithActor(seat, automatic), "gmail", &off); err == nil {
		t.Error("automatic work changed a mailbox setting")
	}

	if err := env.registry.Disconnect(agent, "gmail"); err != nil {
		t.Fatalf("disconnecting: %v", err)
	}
	var status string
	if err := database.WithWorkspaceTx(agent, env.e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(agent, `SELECT status FROM capture_connection WHERE id = $1`, views[0].ID).Scan(&status)
	}); err != nil {
		t.Fatalf("reading the connection: %v", err)
	}
	if status != "disconnected" {
		t.Errorf("the mailbox after the agent's disconnect is %q, want disconnected", status)
	}
}
