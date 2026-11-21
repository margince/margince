// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package apptest

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A gate's refusal reaches an agent in the taxonomy's words. A role sweep reads
// it as a 403, not as an answer no gate decided.
func TestAGateRefusalInTheAgentsWordsIsARefusedVerdict(t *testing.T) {
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
	answer := MCPResult{Text: agents.RunRefusal(refusal), IsError: true}

	if !AnswerRefused(answer.Text) {
		t.Errorf("AnswerRefused(%q) = false, want the gate's refusal read as a 403", answer.Text)
	}
	if got := VerdictOf(answer, []string{"tag.create"}); got != VerdictRefused {
		t.Errorf("VerdictOf(%q) = %v, want VerdictRefused", answer.Text, got)
	}
	unrecorded := MCPResult{Text: agents.RunRefusal(errors.New("store fault")), IsError: true}
	if got := VerdictOf(unrecorded, []string{"tag.create"}); got != VerdictOpen {
		t.Errorf("VerdictOf(%q) = %v, want VerdictOpen for an answer that is no 403", unrecorded.Text, got)
	}
}
