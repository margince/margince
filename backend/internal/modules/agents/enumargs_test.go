// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// carriedEnum is a property carrying an `enum` of any kind. It is read with no
// `type` filter, so the walk sees what the registry may skip.
type carriedEnum struct {
	name string
	// words is nil when the enum is not all text.
	words []string
	list  bool
}

func carriedEnums(t *testing.T, tool string, inputSchema json.RawMessage) []carriedEnum {
	t.Helper()
	var schema struct {
		Properties map[string]struct {
			Enum  []json.RawMessage `json:"enum"`
			Items *struct {
				Enum []json.RawMessage `json:"enum"`
			} `json:"items"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(inputSchema, &schema); err != nil {
		t.Fatalf("%s: inputSchema does not parse: %v", tool, err)
	}
	var out []carriedEnum
	for name, prop := range schema.Properties {
		members, list := prop.Enum, false
		if len(members) == 0 && prop.Items != nil {
			members, list = prop.Items.Enum, true
		}
		if len(members) == 0 {
			continue
		}
		words, _ := textWords(members)
		out = append(out, carriedEnum{name: name, words: words, list: list})
	}
	return out
}

// A word outside a declared vocabulary is refused by name on every tool that
// declares one. An empty string is not taken for an absent argument.
func TestEveryDeclaredVocabularyBindsTheToolThatDeclaresIt(t *testing.T) {
	registry := idProbeDispatcher(t).registry
	ctx := scopedAgentCtx(principal.ScopeRead, principal.ScopeDraft,
		principal.ScopeWrite, principal.ScopeSend, principal.ScopeEnrich)

	probed := 0
	for name, tool := range registry.tools {
		for _, enum := range carriedEnums(t, name, tool.Spec().InputSchema) {
			if enum.words == nil {
				continue
			}
			held := slices.ContainsFunc(registry.enumArgs[name], func(e enumArg) bool { return e.name == enum.name })
			if !held {
				t.Errorf("%s declares a vocabulary for %q that the registry does not hold", name, enum.name)
				continue
			}
			for _, word := range []string{"zz_not_a_word", ""} {
				probed++
				var args map[string]any
				if err := json.Unmarshal(absentIDArgs(t, name, tool.Spec().InputSchema, enum.name), &args); err != nil {
					t.Fatalf("%s: probe arguments do not parse: %v", name, err)
				}
				args[enum.name] = word
				if enum.list {
					args[enum.name] = []string{word}
				}
				encoded, err := json.Marshal(args)
				if err != nil {
					t.Fatalf("%s: marshal probe: %v", name, err)
				}

				_, err = registry.Invoke(ctx, name, encoded)

				var badArgs *BadArgsError
				if !errors.As(err, &badArgs) {
					t.Errorf("%s accepted %q for %q, outside its declared words %v (answered %T: %v)",
						name, word, enum.name, enum.words, err, err)
					continue
				}
				if !strings.Contains(badArgs.Error(), enum.name) || !strings.Contains(badArgs.Error(), enum.words[0]) {
					t.Errorf("%s refused %q=%q without naming the argument and the words it takes: %q",
						name, enum.name, word, badArgs.Error())
				}
			}
		}
	}
	if probed == 0 {
		t.Fatal("no tool declares a vocabulary, so the walk proved nothing")
	}
}

// A vocabulary declared without a `type` is held like any other.
func TestAnUnknownScopeKindIsRefusedByName(t *testing.T) {
	registry := idProbeDispatcher(t).registry
	ctx := scopedAgentCtx(principal.ScopeRead)

	var args map[string]any
	base := absentIDArgs(t, "run_analytics_query", registry.tools["run_analytics_query"].Spec().InputSchema, "scope_kind")
	if err := json.Unmarshal(base, &args); err != nil {
		t.Fatalf("probe arguments do not parse: %v", err)
	}
	args["scope_kind"] = "galaxy"
	encoded, err := json.Marshal(args)
	if err != nil {
		t.Fatalf("marshal probe: %v", err)
	}

	_, err = registry.Invoke(ctx, "run_analytics_query", encoded)

	var badArgs *BadArgsError
	if !errors.As(err, &badArgs) || badArgs.Field != "scope_kind" || !strings.Contains(badArgs.Error(), "workspace") {
		t.Errorf("an unknown scope_kind answered %T (%v), want a refusal naming scope_kind and its words", err, err)
	}
}
