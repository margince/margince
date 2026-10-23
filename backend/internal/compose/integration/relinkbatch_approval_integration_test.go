// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// What automatic work's batch relink onto a project answers. It may not stage the
// approval a project destination asks for. Both the thread form and the named
// set are refused and nothing is filed.

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// relinkAgentCtx is an agent principal lent by Rep1, holding the write scope. A
// passport must be a real row, because a staged approval is foreign-keyed to it.
// A zero passport is automatic work, which a confirm-first tier refuses.
func relinkAgentCtx(e *Env, passport ids.UUID) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalAgent, ID: "agent:relink-probe", SeatType: principal.SeatFull,
		OnBehalfOf: e.Rep1, UserID: e.Rep1, PassportID: passport,
		Scopes:      principal.NewScopeSet(principal.ScopeRead, principal.ScopeWrite),
		Permissions: AdminPerms,
	})
}

func TestAutomaticWorkIsRefusedABatchRelinkOntoAProject(t *testing.T) {
	e := Setup(t)
	owner := OwnerConn(t)
	f := seedThreadFixture(t, e, owner)
	registry := compose.NewRegistry(e.Pool, compose.SendPath{})
	// The gate re-derives the lender's LIVE role from the database, and this
	// harness seeds users but no roles at all.
	e.WsExec(t, `INSERT INTO role (key, name, permissions) VALUES ('relink-lender', 'Relink lender', $1::jsonb)`,
		`{"objects":{"activity":{"read":true,"update":true},"project":{"read":true},"contact":{"read":true}},"row_scope":"all"}`)
	e.WsExec(t, `INSERT INTO role_assignment (role_id, user_id) SELECT r.id, $1 FROM role r WHERE r.key = 'relink-lender'`, e.Rep1)
	agent := relinkAgentCtx(e, ids.UUID{})

	// The thread form is refused before any staging. A key names a conversation
	// that may grow, so the refusal sends the caller to the named set.
	_, err := registry.Invoke(agent, "relink_thread",
		json.RawMessage(`{"thread_key":"`+f.key+`","entity_type":"project","entity_id":"`+f.project.String()+`"}`))
	if _, ok := errors.AsType[*agents.BadArgsError](err); !ok {
		t.Fatalf("relink_thread onto a project → %v, want a refusal naming relink_activities", err)
	}

	// The named set would need an approval, which automatic work may not stage.
	args := `{"activity_ids":["` + f.mine[0].String() + `","` + f.mine[1].String() +
		`"],"entity_type":"project","entity_id":"` + f.project.String() + `"}`
	_, err = registry.Invoke(agent, "relink_activities", json.RawMessage(args))
	if !isPassportlessStagingRefusal(err) {
		t.Fatalf("relink_activities onto a project → %v, want the refusal that automatic work cannot stage", err)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM approval`); n != 0 {
		t.Errorf("automatic work left %d approval(s), want none", n)
	}
	for _, id := range f.mine {
		if projectLinks(t, e, id, f.project) != 0 {
			t.Errorf("activity %s was filed under the project by a refused call", id)
		}
	}
}
