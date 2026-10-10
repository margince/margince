// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// The closed vocabulary a tool's schema declares for a text argument, and its
// enforcement: the sibling of numargs.go, at the same chokepoint and for the
// same reason. An `enum` in a tools/list entry is the only list of words a
// caller has, so a word outside it must be refused by name rather than read as
// "nothing matches" (an empty list for a made-up record type) or accepted as
// though it were absent (an empty string).
//
// Scope: top-level string properties, and top-level arrays of strings, that
// declare an `enum`. A property whose vocabulary is not all text is skipped.

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
			Type  string   `json:"type"`
			Enum  []string `json:"enum"`
			Items *struct {
				Enum []string `json:"enum"`
			} `json:"items"`
		} `json:"properties"`
	}
	// An unreadable vocabulary (not all text) leaves that tool unenforced rather
	// than failing boot: numeric and mixed enums are outside this claim.
	if json.Unmarshal(inputSchema, &schema) != nil {
		return nil
	}
	var out []enumArg
	for name, prop := range schema.Properties {
		switch {
		case prop.Type == schemaString && len(prop.Enum) > 0:
			out = append(out, enumArg{name: name, words: prop.Enum})
		case prop.Type == schemaArray && prop.Items != nil && len(prop.Items.Enum) > 0:
			out = append(out, enumArg{name: name, words: prop.Items.Enum, list: true})
		}
	}
	// Sorted, so a call breaking two vocabularies is refused in the same words
	// every time rather than in Go's map order.
	sort.Slice(out, func(i, j int) bool { return out[i].name < out[j].name })
	return out
}

// requireDeclaredEnums holds every supplied word to the vocabulary its tool
// advertises for it.
//
// An ABSENT argument and an explicit null are legal, as for the bounds. An empty
// string is NOT absent: it is a word, and not one of the declared ones.
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
	// The vocabulary rides in Guidance, which is ours and unbounded: a long list
	// in the bounded Cause would be cut off exactly when the caller needs it.
	bad := &BadArgsError{Cause: errors.New(strings.Join(refusals, "; ")), Guidance: strings.Join(vocabularies, "; ")}
	if len(fields) == 1 {
		bad.Field = fields[0]
	}
	return bad
}

// violation reports whether the supplied value is outside the vocabulary, and
// how to say so. A value that is not text at all is left to the handler's own
// decode, which names the type it wanted.
//
// The word is the caller's and is rendered quoted and bounded by BadArgsError;
// the vocabulary is ours.
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
