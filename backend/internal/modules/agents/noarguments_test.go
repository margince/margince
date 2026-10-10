// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// declaresNoArguments reports whether a schema is the object that admits no
// member at all, read off the schema itself rather than off any tool's code.
func declaresNoArguments(t *testing.T, tool string, inputSchema json.RawMessage) bool {
	t.Helper()
	var schema struct {
		Properties           map[string]json.RawMessage `json:"properties"`
		AdditionalProperties *bool                      `json:"additionalProperties"`
	}
	if err := json.Unmarshal(inputSchema, &schema); err != nil {
		t.Fatalf("%s: inputSchema does not parse: %v", tool, err)
	}
	return len(schema.Properties) == 0 && schema.AdditionalProperties != nil && !*schema.AdditionalProperties
}

// A tool that declares no arguments refuses one by name. Serving a misspelt or
// misplaced member as though it were absent answers a question the caller did
// not ask, in the shape of the right answer.
func TestEveryToolThatDeclaresNoArgumentsRefusesAMemberByName(t *testing.T) {
	registry := idProbeDispatcher(t).registry
	ctx := scopedAgentCtx(principal.ScopeRead, principal.ScopeDraft,
		principal.ScopeWrite, principal.ScopeSend, principal.ScopeEnrich)

	probed := 0
	for name, tool := range registry.tools {
		if !declaresNoArguments(t, name, tool.Spec().InputSchema) {
			continue
		}
		probed++
		_, err := registry.Invoke(ctx, name, json.RawMessage(`{"zz_unknown":1}`))

		var badArgs *BadArgsError
		if !errors.As(err, &badArgs) {
			t.Errorf("%s answered an undeclared member with %T (%v), want *BadArgsError", name, err, err)
			continue
		}
		if !strings.Contains(badArgs.Error(), "zz_unknown") {
			t.Errorf("%s refused the call without naming the member: %q", name, badArgs.Error())
		}
	}
	// A surface that stopped declaring such tools would pass by probing nothing.
	if probed == 0 {
		t.Fatal("no registered tool declares an empty argument object, so the walk proved nothing")
	}
}
