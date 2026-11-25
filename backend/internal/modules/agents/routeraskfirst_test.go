// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// argumentRouters are the served routers with no action field: a client steers
// them by which arguments it names. Read off the registry, so a new one joins.
func argumentRouters(t *testing.T) []mcp.ToolSpec {
	t.Helper()
	var routers []mcp.ToolSpec
	for _, spec := range fullRegistry(t).ListedSpecs() {
		if len(spec.Actions) > 0 && spec.ActionField == "" {
			routers = append(routers, spec)
		}
	}
	if len(routers) < 2 {
		t.Fatalf("found %d routers without an action field; the census reads short", len(routers))
	}
	return routers
}

// Each choice of such a router names arguments its own schema takes, so the
// ask-first note sends the client to words it can send.
func TestARouterWithoutAnActionFieldNamesTheArgumentsThatPickEachTarget(t *testing.T) {
	for _, spec := range argumentRouters(t) {
		var schema struct {
			Properties map[string]json.RawMessage `json:"properties"`
		}
		if err := json.Unmarshal(spec.InputSchema, &schema); err != nil {
			t.Fatalf("%s: %v", spec.Name, err)
		}
		for _, action := range spec.Actions {
			if action.PickedBy == "" {
				t.Errorf("%s: choice %s names no argument that picks it", spec.Name, action.Action)
			}
			for word := range strings.FieldsSeq(action.PickedBy) {
				if _, ok := schema.Properties[word]; !ok && word != "or" && word != "and" {
					t.Errorf("%s: choice %s is picked by %q, which the router's schema does not take", spec.Name, action.Action, word)
				}
			}
		}
	}
}

// The note never names a target key: no argument takes one as its value.
func TestTheAskFirstNoteOfAnArgumentRouterNamesNoTargetKey(t *testing.T) {
	for _, spec := range argumentRouters(t) {
		note := askUserFirst(spec)
		if strings.Contains(note, "for the action") {
			t.Errorf("%s: the note %q names an action a client cannot send", spec.Name, note)
		}
	}
}

func TestAnArgumentRouterAsksFirstAsItsTargetsDo(t *testing.T) {
	r := fullRegistry(t)
	for name, want := range map[string]string{
		"decide_approvals":   askFirstOpening,
		"resolve_duplicates": askFirstOpening + " when you name source_id or contact_id, unless preview is true",
		"read_relationships": "",
	} {
		spec, ok := r.Spec(name)
		if !ok {
			t.Fatalf("%s is not registered", name)
		}
		if got := askUserFirst(spec); got != want {
			t.Errorf("%s: the ask-first note reads %q, want %q", name, got, want)
		}
	}
}
