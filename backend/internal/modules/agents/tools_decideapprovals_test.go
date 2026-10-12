// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/platform/auth"
)

func TestDecideApprovalsPicksTheTargetByTheIDItNames(t *testing.T) {
	r := NewRegistry(nil, auth.NewGate(fullSeatAuthority{}))
	RegisterApprovalTools(r, &fakeInbox{})
	for _, tc := range []struct{ args, target string }{
		{`{"staged_action_id":"not-a-uuid","decision":"approve"}`, "decide_approval"},
		{`{"bundle_id":"not-a-uuid","decision":"approve"}`, "decide_approval_bundle"},
	} {
		served, err := r.InvokeServed(agentCtx(), "decide_approvals", json.RawMessage(tc.args))
		if _, bad := errors.AsType[*BadArgsError](err); !bad || served.Tool != tc.target {
			t.Errorf("%s: served %q with %v, want %s's own refusal", tc.args, served.Tool, err, tc.target)
		}
	}
	for _, args := range []string{
		`{"decision":"approve"}`,
		`{"staged_action_id":"a","bundle_id":"b","decision":"approve"}`,
	} {
		served, err := r.InvokeServed(agentCtx(), "decide_approvals", json.RawMessage(args))
		if _, bad := errors.AsType[*BadArgsError](err); !bad || served.Tool != "decide_approvals" {
			t.Errorf("%s: served %q with %v, want the router's refusal naming exactly one id", args, served.Tool, err)
		}
	}
}
