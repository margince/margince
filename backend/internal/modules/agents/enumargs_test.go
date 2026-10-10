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

// declaredEnums reads the closed vocabularies a tool's own schema states,
// without asking the registry what it enforces.
func declaredEnums(t *testing.T, tool string, inputSchema json.RawMessage) []enumArg {
	t.Helper()
	var schema struct {
		Properties map[string]struct {
			Type  string   `json:"type"`
			Enum  []string `json:"enum"`
			Items *struct {
				Enum []string `json:"enum"`
			} `json:"items"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(inputSchema, &schema); err != nil {
		t.Fatalf("%s: inputSchema does not parse: %v", tool, err)
	}
	var out []enumArg
	for name, prop := range schema.Properties {
		switch {
		case prop.Type == "string" && len(prop.Enum) > 0:
			out = append(out, enumArg{name: name, words: prop.Enum})
		case prop.Type == "array" && prop.Items != nil && len(prop.Items.Enum) > 0:
			out = append(out, enumArg{name: name, words: prop.Items.Enum, list: true})
		}
	}
	return out
}

// A word outside a declared vocabulary is refused by name on every tool that
// declares one, and an empty string is not read as an absent argument. Without
// it read_lists answered a made-up record type with an empty list.
func TestEveryDeclaredVocabularyBindsTheToolThatDeclaresIt(t *testing.T) {
	registry := idProbeDispatcher(t).registry
	ctx := scopedAgentCtx(principal.ScopeRead, principal.ScopeDraft,
		principal.ScopeWrite, principal.ScopeSend, principal.ScopeEnrich)

	probed := 0
	for name, tool := range registry.tools {
		for _, enum := range declaredEnums(t, name, tool.Spec().InputSchema) {
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
