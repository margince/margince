// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/pkg/extension"
)

// TestWriteVerbLiteralsEmitsHumanOnlyOnlyWhenTrue: HumanOnly follows Subject's
// own convention — emitted only when set, so a forgotten line in a future edit
// reads as a failing assertion here rather than a silently-false field at
// boot.
func TestWriteVerbLiteralsEmitsHumanOnlyOnlyWhenTrue(t *testing.T) {
	verbs := []declaredVerb{
		{verb: extension.Verb{
			Unit: "u", Contract: "crm.yaml", OperationID: "uHuman", Route: "/ext/u/human",
			Method: "POST", Tool: "u_human", Title: "t", Description: "d", Version: "1.0.0",
			HumanOnly: true, RbacObject: "ext_u_widget", RbacAction: extension.RbacUpdate,
		}},
		{verb: extension.Verb{
			Unit: "u", Contract: "crm.yaml", OperationID: "uAgent", Route: "/ext/u/agent",
			Method: "POST", Tool: "u_agent", Title: "t", Description: "d", Version: "1.0.0",
			Tier: extension.TierAutoExecute, RequestedScope: extension.ScopeRead,
		}},
	}
	var b strings.Builder
	writeVerbLiterals(&b, verbs)
	got := b.String()

	humanOnlySection := got[:strings.Index(got, "uAgent")]
	if !strings.Contains(humanOnlySection, "HumanOnly:      true,") {
		t.Fatalf("the human-only verb's literal must carry HumanOnly: true, got:\n%s", humanOnlySection)
	}
	agentSection := got[strings.Index(got, "uAgent"):]
	if strings.Contains(agentSection, "HumanOnly") {
		t.Fatalf("a non-human-only verb's literal must carry no HumanOnly line, got:\n%s", agentSection)
	}
}
