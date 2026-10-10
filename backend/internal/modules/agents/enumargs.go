// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// A text argument's declared `enum`, held at the chokepoint numargs.go uses.
// A word outside it is refused by name, whatever the property's `type` says.

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
)

// enumArg is one text argument's closed vocabulary.
type enumArg struct {
	name  string
	words []string
	// list is true when the argument is an array whose items carry the enum.
	list bool
}

// declaredEnumArgs reads a schema's text vocabularies once, at registration.
func declaredEnumArgs(inputSchema json.RawMessage) []enumArg {
	var schema struct {
		Properties map[string]struct {
			Enum  []json.RawMessage `json:"enum"`
			Items *struct {
				Enum []json.RawMessage `json:"enum"`
			} `json:"items"`
		} `json:"properties"`
	}
	if json.Unmarshal(inputSchema, &schema) != nil {
		return nil
	}
	var out []enumArg
	for name, prop := range schema.Properties {
		if words, ok := textWords(prop.Enum); ok {
			out = append(out, enumArg{name: name, words: words})
		} else if prop.Items != nil {
			if words, ok := textWords(prop.Items.Enum); ok {
				out = append(out, enumArg{name: name, words: words, list: true})
			}
		}
	}
	// Sorted, so a call breaking two vocabularies is refused in the same words
	// every time rather than in Go's map order.
	sort.Slice(out, func(i, j int) bool { return out[i].name < out[j].name })
	return out
}

// textWords reads an enum whose members are all strings; any other enum is not
// a text vocabulary and stays unenforced.
func textWords(members []json.RawMessage) ([]string, bool) {
	words := make([]string, 0, len(members))
	for _, member := range members {
		var word string
		if json.Unmarshal(member, &word) != nil {
			return nil, false
		}
		words = append(words, word)
	}
	return words, len(words) > 0
}

// requireDeclaredEnums refuses a word outside the declared vocabulary. An empty
// string is a word, so it is refused; absent and null are legal.
func (r *Registry) requireDeclaredEnums(name string, args json.RawMessage) error {
	r.mu.RLock()
	enums := r.enumArgs[name]
	r.mu.RUnlock()
	if len(enums) == 0 {
		return nil
	}
	present, isObject := argsAsObject(args)
	if !isObject {
		return nil
	}
	var refusals, vocabularies, fields []string
	for _, enum := range enums {
		raw, supplied := present[enum.name]
		if !supplied {
			continue
		}
		if refusal, broken := enum.violation(raw); broken {
			refusals = append(refusals, refusal)
			vocabularies = append(vocabularies, fmt.Sprintf("`%s` takes one of: %s", enum.name, strings.Join(enum.words, ", ")))
			fields = append(fields, enum.name)
		}
	}
	if len(refusals) == 0 {
		return nil
	}
	// The vocabulary rides in Guidance, which is ours and unbounded. A long list
	// in the bounded Cause would be cut off when the caller needs it.
	bad := &BadArgsError{Cause: errors.New(strings.Join(refusals, "; ")), Guidance: strings.Join(vocabularies, "; ")}
	if len(fields) == 1 {
		bad.Field = fields[0]
	}
	return bad
}

// violation reports whether the supplied value is outside the vocabulary. A
// value that is not text is left to the handler's own decode, which names the
// type it wanted. The word is the caller's and BadArgsError bounds it.
func (e enumArg) violation(raw json.RawMessage) (string, bool) {
	if e.list {
		var words []string
		if json.Unmarshal(raw, &words) != nil {
			return "", false
		}
		for _, word := range words {
			if !slices.Contains(e.words, word) {
				return e.refusal(word), true
			}
		}
		return "", false
	}
	var word *string
	if json.Unmarshal(raw, &word) != nil || word == nil {
		return "", false
	}
	if slices.Contains(e.words, *word) {
		return "", false
	}
	return e.refusal(*word), true
}

func (e enumArg) refusal(word string) string {
	return fmt.Sprintf("`%s` is %q, which is not a declared word", e.name, word)
}
