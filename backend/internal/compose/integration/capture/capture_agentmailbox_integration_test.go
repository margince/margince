// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	capturemod "github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
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

// An agent on its user's passport lists the connections its user does, no more,
// and the list carries no credential. Automatic work with no passport is
// refused.
func TestAnAgentListsItsUsersConnectionsAndNoCredential(t *testing.T) {
	env := newCaptureEnv(t)
	seat := seatContext(env.e, env.e.Rep1)
	agent := agentOfSeat(seat, t)

	human, err := env.registry.Connections(seat)
	if err != nil || len(human) == 0 {
		t.Fatalf("the user listing their mailboxes = %d (%v), want the one", len(human), err)
	}
	listed, err := env.registry.Connections(agent)
	if err != nil {
		t.Fatalf("the agent listing its user's mailboxes: %v", err)
	}
	humanJSON, agentJSON := mustJSON(t, human), mustJSON(t, listed)
	if humanJSON != agentJSON {
		t.Errorf("the agent was listed\n%s\nand its user\n%s", agentJSON, humanJSON)
	}
	var reference, token string
	if err := database.WithWorkspaceTx(seat, env.e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(seat, `SELECT coalesce(credential_ref, ''), coalesce(encode(auth, 'base64'), '')
			FROM capture_connection WHERE id = $1`, human[0].ID).Scan(&reference, &token)
	}); err != nil {
		t.Fatalf("reading the connection's credential: %v", err)
	}
	if reference == "" && token == "" {
		t.Fatal("the seeded connection holds no credential, so nothing below proves the list leaves it out")
	}
	for _, secret := range []string{reference, token} {
		if secret != "" && strings.Contains(agentJSON, secret) {
			t.Errorf("the agent's list carries the connection's credential %q", secret)
		}
	}

	automatic, _ := principal.Actor(agent)
	automatic.PassportID = ids.UUID{}
	if _, err := env.registry.Connections(principal.WithActor(seat, automatic)); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("automatic work listing a user's mailboxes = %v, want permission denied", err)
	}
}

func mustJSON(t *testing.T, v []capturemod.ConnectionView) string {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
