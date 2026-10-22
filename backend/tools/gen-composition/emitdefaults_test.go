// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/pkg/extension"
)

// A composed binary ships no repository, so default grants reach it as a
// literal. A field the emitter drops leaves a unit nobody can use, silently.
func TestWriteVerbLiteralsEmitsTheDefaultGrantsOnlyWhenDeclared(t *testing.T) {
	verbs := []declaredVerb{
		{verb: extension.Verb{
			Unit: "u", Contract: "crm.yaml", OperationID: "uGated", Route: "/ext/u/gated",
			Method: "GET", Tool: "u_gated", Title: "t", Description: "d", Version: "1.0.0",
			Tier: extension.TierAutoExecute, RequestedScope: extension.ScopeRead,
			RbacObject: "ext_u_widget", RbacAction: extension.RbacRead,
			RbacDefaults: []extension.RbacDefault{
				{Role: "admin", Actions: []extension.RbacAction{extension.RbacRead, extension.RbacUpdate}},
				{Role: "rep", Actions: []extension.RbacAction{extension.RbacRead}},
			},
		}},
		{verb: extension.Verb{
			Unit: "u", Contract: "crm.yaml", OperationID: "uPlain", Route: "/ext/u/plain",
			Method: "GET", Tool: "u_plain", Title: "t", Description: "d", Version: "1.0.0",
			Tier: extension.TierAutoExecute, RequestedScope: extension.ScopeRead,
		}},
	}
	var b strings.Builder
	writeVerbLiterals(&b, verbs)
	got := b.String()

	gated := got[:strings.Index(got, "uPlain")]
	for _, want := range []string{
		"RbacDefaults:",
		`{Role: "admin", Actions: []extension.RbacAction{"read", "update"}}`,
		`{Role: "rep", Actions: []extension.RbacAction{"read"}}`,
	} {
		if !strings.Contains(gated, want) {
			t.Errorf("the gated verb's literal lacks %q, got:\n%s", want, gated)
		}
	}
	if plain := got[strings.Index(got, "uPlain"):]; strings.Contains(plain, "RbacDefaults") {
		t.Errorf("a verb with no default grants must carry no RbacDefaults line, got:\n%s", plain)
	}
}
