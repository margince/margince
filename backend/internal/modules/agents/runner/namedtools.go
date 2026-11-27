// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package runner

import (
	"bytes"
	"encoding/json"
	"regexp"
	"slices"
	"strings"
	"unicode"

	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// sentenceEnd is where a sentence of tool copy stops: a stop and the space
// after it. A sentence may begin with a tool name, so the next letter is no
// test; abbreviationEnds names the stops that end no sentence.
var (
	sentenceEnd      = regexp.MustCompile(`([.!?])\s+`)
	abbreviationEnds = []string{"e.g.", "i.e.", "etc.", "vs."}
)

// withoutSentencesNaming drops, from the description and from each description
// in the input schema, every sentence that names a registered tool this run is
// not offered. A pointer to a tool the allowlist refuses only costs a step.
func withoutSentencesNaming(spec mcp.ToolSpec, offered map[string]bool) mcp.ToolSpec {
	var gone []string
	for _, name := range spec.NamedTools {
		if !offered[name] {
			gone = append(gone, name)
		}
	}
	if len(gone) == 0 {
		return spec
	}
	spec.Description = dropSentencesNaming(spec.Description, gone)
	var schema any
	if err := json.Unmarshal(spec.InputSchema, &schema); err != nil {
		return spec
	}
	if !dropFromDescriptions(schema, gone) {
		return spec
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(schema); err != nil {
		return spec
	}
	spec.InputSchema = json.RawMessage(bytes.TrimSpace(buf.Bytes()))
	return spec
}

// dropFromDescriptions rewrites every "description" string under node and
// reports whether any changed.
//craft:ignore naked-any a decoded JSON schema is any by construction; this walk reads its maps, slices and strings
func dropFromDescriptions(node any, gone []string) bool {
	changed := false
	switch v := node.(type) {
	case map[string]any:
		for key, child := range v {
			if text, ok := child.(string); ok && key == "description" {
				if kept := dropSentencesNaming(text, gone); kept != text {
					changed = true
					if kept == "" {
						delete(v, key)
					} else {
						v[key] = kept
					}
				}
				continue
			}
			if dropFromDescriptions(child, gone) {
				changed = true
			}
		}
	case []any:
		for _, child := range v {
			if dropFromDescriptions(child, gone) {
				changed = true
			}
		}
	}
	return changed
}

// dropSentencesNaming keeps the sentences of text that name none of the tools.
func dropSentencesNaming(text string, gone []string) string {
	var kept []string
	dropped := false
	start := 0
	flush := func(end int) {
		sentence := strings.TrimSpace(text[start:end])
		switch {
		case sentence == "":
		case namesAny(sentence, gone):
			dropped = true
		default:
			kept = append(kept, sentence)
		}
	}
	for _, loc := range sentenceEnd.FindAllStringSubmatchIndex(text, -1) {
		if endsAbbreviation(text[:loc[3]]) {
			continue
		}
		flush(loc[3])
		start = loc[1]
	}
	flush(len(text))
	if !dropped {
		return text
	}
	return strings.Join(kept, " ")
}

func endsAbbreviation(prefix string) bool {
	return slices.ContainsFunc(abbreviationEnds, func(a string) bool { return strings.HasSuffix(prefix, a) })
}

func namesAny(sentence string, names []string) bool {
	for _, word := range strings.FieldsFunc(sentence, func(c rune) bool {
		return c != '_' && !unicode.IsLower(c) && !unicode.IsDigit(c)
	}) {
		if slices.Contains(names, word) {
			return true
		}
	}
	return false
}
