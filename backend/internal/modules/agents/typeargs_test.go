// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// typedStaging is the confirm-first stagingTool with typed arguments.
type typedStaging struct{ *stagingTool }

func (s typedStaging) Spec() mcp.ToolSpec {
	spec := s.stagingTool.Spec()
	spec.InputSchema = json.RawMessage(`{"type":"object","properties":{
		"reason":{"type":"string"},"count":{"type":"integer"},"weight":{"type":"number"}}}`)
	return spec
}

func TestAWronglyTypedArgumentIsRefusedBeforeAnyoneIsAskedToApproveIt(t *testing.T) {
	approvals := &recordingStageApprovals{}
	r := NewRegistry(approvals, auth.NewGate(fullSeatAuthority{}))
	r.Register(typedStaging{&stagingTool{target: ids.NewV7()}})

	_, err := r.Invoke(agentCtx(), "send_it", json.RawMessage(`{"reason":42,"count":1.5}`))
	bad, refused := errors.AsType[*BadArgsError](err)
	if !refused {
		t.Fatalf("answered %v, want a bad-arguments refusal", err)
	}
	if approvals.kind != "" {
		t.Fatalf("staged %q for a call its handler cannot run", approvals.kind)
	}
	for _, want := range []string{"`reason` is integer", "`count` is number"} {
		if !strings.Contains(bad.Error(), want) {
			t.Errorf("refusal %q does not say %s", bad, want)
		}
	}

	_, err = r.Invoke(agentCtx(), "send_it", json.RawMessage(`{"reason":"duplicate","count":2,"weight":3}`))
	if !errors.Is(err, apperrors.ErrRequiresApproval) || approvals.kind != "send_it" {
		t.Fatalf("a well-typed call answered %v and staged %q, want it staged", err, approvals.kind)
	}
}
