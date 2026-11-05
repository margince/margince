// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"encoding/json"
	"regexp"
	"slices"
	"strings"

	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// hiddenActions are the action words of all that kept leaves out. Only a word
// with an underscore is held to in prose, as a plain word such as "merge" is
// English first.
func hiddenActions(all, kept []mcp.ToolAction) []string {
	var out []string
	for _, action := range all {
		if strings.Contains(action.Action, "_") && !slices.ContainsFunc(kept, func(k mcp.ToolAction) bool { return k.Action == action.Action }) {
			out = append(out, action.Action)
		}
	}
	return out
}

// actionTargets are the registered specs of the verbs actions route to.
func (r *Registry) actionTargets(actions []mcp.ToolAction) []mcp.ToolSpec {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]mcp.ToolSpec, 0, len(actions))
	for _, action := range actions {
		if target, ok := r.tools[action.Verb]; ok {
			out = append(out, target.Spec())
		}
	}
	return out
}

// sentenceEnd ends a sentence of tool copy.
var sentenceEnd = regexp.MustCompile(`[.!?](\s+|$)`)

// withoutSentencesNaming is text less each sentence that names a hidden action.
func withoutSentencesNaming(text string, hidden []string) string {
	if len(hidden) == 0 || text == "" {
		return text
	}
	names := namesAny(hidden)
	var kept []string
	start := 0
	for _, end := range sentenceEnd.FindAllStringIndex(text, -1) {
		if sentence := strings.TrimSpace(text[start:end[1]]); !names(sentence) {
			kept = append(kept, sentence)
		}
		start = end[1]
	}
	if rest := strings.TrimSpace(text[start:]); rest != "" && !names(rest) {
		kept = append(kept, rest)
	}
	return strings.Join(kept, " ")
}

// clauseEnd parts the clauses a route field's description gives each action.
var clauseEnd = regexp.MustCompile(`[;,]\s+`)

// withoutClausesNaming is a route field's schema with its description less each
// clause that names a hidden action.
func withoutClausesNaming(field json.RawMessage, hidden []string) (json.RawMessage, error) {
	if len(hidden) == 0 || len(field) == 0 {
		return field, nil
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(field, &doc); err != nil {
		return nil, err
	}
	raw, has := doc["description"]
	if !has {
		return field, nil
	}
	var description string
	if err := json.Unmarshal(raw, &description); err != nil {
		return nil, err
	}
	names := namesAny(hidden)
	clauses := slices.DeleteFunc(clauseEnd.Split(description, -1), names)
	var err error
	if doc["description"], err = json.Marshal(strings.Join(clauses, "; ")); err != nil {
		return nil, err
	}
	return json.Marshal(doc)
}

// namesAny reports whether a text names any of words as a whole word.
func namesAny(words []string) func(string) bool {
	quoted := make([]string, len(words))
	for i, word := range words {
		quoted[i] = regexp.QuoteMeta(word)
	}
	pattern := regexp.MustCompile(`\b(` + strings.Join(quoted, "|") + `)\b`)
	return pattern.MatchString
}
