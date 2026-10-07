// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package approvals

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// What an agent is told it can release is what decide would let it release: the
// classification says undoable, the session is attended, the credential carries
// write and the decision grants, and the target is one it may act on.
func TestAStagedCallIsReleasableByTheCallerOnlyWhenDecideWouldAllowIt(t *testing.T) {
	e := setupStaging(t)
	target := ids.NewV7()
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO company (id, display_name, source, captured_by)
		VALUES ($1, 'Releasable', 'gmail:seed', 'connector:gmail')`, target); err != nil {
		t.Fatalf("seeding the target: %v", err)
	}
	change := json.RawMessage(`{"proposed_name":"Releasable Global"}`)
	undoable := func(_ context.Context, _ Queryer, call StagedCall) bool {
		return call.Kind == "company_name_promotion" && call.TargetID == target
	}
	agent := e.lentPassport(t, e.rep)
	actor, _ := principal.Actor(agent)
	with := func(mutate func(*principal.Principal)) context.Context {
		p := actor
		mutate(&p)
		return principal.WithActor(agent, p)
	}

	for _, tc := range []struct {
		name string
		svc  *Service
		ctx  context.Context
		want bool
	}{
		{"an undoable call, attended, with write and the grants", e.svc.WithUndoableRelease(undoable), agent, true},
		{"a call that is not undoable", NewService(e.svc.db).WithUndoableRelease(func(context.Context, Queryer, StagedCall) bool { return false }), agent, false},
		{"an unattended run", NewService(e.svc.db).WithUndoableRelease(undoable), principal.WithAgentRunID(agent, ids.NewV7()), false},
		{"no classification installed", NewService(e.svc.db), agent, false},
		{
			"a credential without the write cap", NewService(e.svc.db).WithUndoableRelease(undoable),
			with(func(p *principal.Principal) { p.Scopes = principal.NewScopeSet(principal.ScopeRead) }), false,
		},
		{
			"a credential without the decision grant", NewService(e.svc.db).WithUndoableRelease(undoable),
			with(func(p *principal.Principal) { p.Permissions = principal.Permissions{RowScope: principal.RowScopeAll} }), false,
		},
		{
			"a credential acting for nobody", NewService(e.svc.db).WithUndoableRelease(undoable),
			with(func(p *principal.Principal) { p.OnBehalfOf, p.UserID = ids.UUID{}, ids.UUID{} }), false,
		},
		{"no actor on the context", NewService(e.svc.db).WithUndoableRelease(undoable), context.Background(), false},
	} {
		if got := tc.svc.ReleasableByCaller(tc.ctx, "company_name_promotion", tableCompany, target, change); got != tc.want {
			t.Errorf("%s: ReleasableByCaller = %v, want %v", tc.name, got, tc.want)
		}
	}
}
