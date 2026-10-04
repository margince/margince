// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// What an agent is told it can release is what decide would let it release: the
// classification says undoable, the session is attended, and the credential
// carries write and acts for the human the proposal is staged for.
func TestAStagedCallIsReleasableByTheCallerOnlyWhenDecideWouldAllowIt(t *testing.T) {
	company := json.RawMessage(`{"entity_type":"company"}`)
	undoableCompanyOnly := func(_, _ string, change json.RawMessage) bool { return string(change) == string(company) }
	lender := ids.NewV7()
	agent := func(scopes ...principal.Scope) principal.Principal {
		return principal.Principal{
			Type: principal.PrincipalAgent, UserID: lender, OnBehalfOf: lender, PassportID: ids.NewV7(),
			Scopes: principal.NewScopeSet(scopes...),
		}
	}
	attended := func(p principal.Principal) context.Context { return principal.WithActor(context.Background(), p) }

	for _, tc := range []struct {
		name   string
		svc    *Service
		ctx    context.Context
		change json.RawMessage
		want   bool
	}{
		{"an undoable call, attended, with write", NewService(nil).WithUndoableRelease(undoableCompanyOnly), attended(agent(principal.ScopeWrite)), company, true},
		{"a call that is not undoable", NewService(nil).WithUndoableRelease(undoableCompanyOnly), attended(agent(principal.ScopeWrite)), json.RawMessage(`{"entity_type":"project"}`), false},
		{
			"an unattended run", NewService(nil).WithUndoableRelease(undoableCompanyOnly),
			principal.WithAgentRunID(attended(agent(principal.ScopeWrite)), ids.NewV7()), company, false,
		},
		{"no classification installed", NewService(nil), attended(agent(principal.ScopeWrite)), company, false},
		{"a credential without the write cap", NewService(nil).WithUndoableRelease(undoableCompanyOnly), attended(agent(principal.ScopeRead)), company, false},
		{"a credential acting for nobody", NewService(nil).WithUndoableRelease(undoableCompanyOnly), attended(func() principal.Principal {
			p := agent(principal.ScopeWrite)
			p.OnBehalfOf, p.UserID = ids.UUID{}, ids.UUID{}
			return p
		}()), company, false},
		{"no actor on the context", NewService(nil).WithUndoableRelease(undoableCompanyOnly), context.Background(), company, false},
	} {
		if got := tc.svc.ReleasableByCaller(tc.ctx, "relink_activities", "company", tc.change); got != tc.want {
			t.Errorf("%s: ReleasableByCaller = %v, want %v", tc.name, got, tc.want)
		}
	}
}
