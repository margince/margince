// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package apptest

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The role sweep reads the dispatcher's answers. A gate's refusal must read as
// a 403 there, and a 422 that names a field's code must not.
func TestTheRoleSweepReadsTheDispatchersRefusalsByStatus(t *testing.T) {
	t.Parallel()
	readOnly := principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:read_only", Permissions: principal.Permissions{
			RoleKeys: []string{"read_only"}, RowScope: principal.RowScopeAll,
			Objects: map[string]principal.ObjectGrant{"tag": {Read: true}},
		},
	})
	refusal := auth.Require(readOnly, "tag", principal.ActionCreate)
	if refusal == nil {
		t.Fatal("a role with only tag.read was admitted to tag.create")
	}
	refused := MCPResult{Text: agents.AuthorityRefusal(refusal), IsError: true}
	if got := VerdictOf(refused, []string{"tag.create"}); got != VerdictRefused {
		t.Errorf("VerdictOf(%q) = %v, want VerdictRefused", refused.Text, got)
	}
	rejected := agents.RunRefusal(httperr.Validation("owner_id", "permission_denied", "pick an owner you may assign"))
	if AnswerRefused(rejected) {
		t.Errorf("AnswerRefused(%q) = true, want a 422 with a field code read as no 403", rejected)
	}
}
