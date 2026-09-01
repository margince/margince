// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"encoding/json"
	"strings"
)

// ReasoningOutputMaxTokens is the output-token cap every structured
// model lane sets on a Request. It carries thinking headroom:
// a reasoning model (Gemini 3.x, o-series) spends output tokens on
// internal thinking BEFORE its answer, and that thinking counts against
// maxOutputTokens — so a cap sized for the answer alone starves the
// answer into a MAX_TOKENS stop with zero visible text. The failure is
// worst on the premium rung (e.g. gemini-3.5-flash), which every V1 task's
// ladder can escalate to. This value is generous enough for any V1 lane's
// answer plus that thinking, still small enough that a runaway completion
// terminates. The aicert lane's default candidate-completion cap derives
// from this same constant — one source for the reasoning-headroom ceiling.
const ReasoningOutputMaxTokens = 8192

// Unfence strips a ```json … ``` code fence some models wrap JSON in, so
// one reduction defines what every downstream shape check and gate
// parses — the callers (enrichment extraction, the brief L2 re-order)
// must not each invent their own trim.
//
// A model asked for structured output usually gives it. When one does not, it
// WRAPS: a sentence before the fence, a sentence after it, an uppercase tag.
// Those are not schema violations, and refusing them costs the caller its
// feature over the model's manners.
//
// The trim is tried FIRST and unchanged, so whatever parses today parses
// identically and no caller's behaviour moves. Only text that does not parse is
// looked at again, for a document the model buried in it.
//
// RECOVERY, NEVER REPAIR. An invalid document stays invalid and its caller
// still refuses it: a reduction that guessed at a missing value would put words
// in the model's mouth, which is worse than the failure it replaces.
func Unfence(text string) string {
	raw := strings.TrimSpace(text)
	trimmed := strings.Trim(strings.TrimPrefix(raw, "```json"), "` \n")
	if json.Valid([]byte(trimmed)) {
		return trimmed
	}
	if buried, found := buriedDocument(raw); found {
		return buried
	}
	return trimmed
}

// buriedDocument answers the largest valid JSON document in text: each fenced
// block, and the span from the first brace to the last.
//
// THE LARGEST, because a model that shows its working writes more than one.
// An illustrative fragment before the answer, or a quoted key after it, is by
// nature smaller than the document it is illustrating — taking the first block
// hands back the example, and taking the last hands back the footnote.
func buriedDocument(text string) (string, bool) {
	best := ""
	for _, candidate := range append(fencedBlocks(text), bracedSpans(text)...) {
		if len(candidate) > len(best) && json.Valid([]byte(candidate)) {
			best = candidate
		}
	}
	return best, best != ""
}

// fencedBlocks answers the contents of every ``` … ``` block in text, with an
// optional language tag on each opening line dropped.
func fencedBlocks(text string) []string {
	var out []string
	rest := text
	for {
		open := strings.Index(rest, "```")
		if open < 0 {
			return out
		}
		rest = rest[open+3:]
		// The language tag lives on the opening line and is not part of the
		// block.
		if newline := strings.IndexByte(rest, '\n'); newline >= 0 {
			if tag := strings.TrimSpace(rest[:newline]); tag != "" && !strings.ContainsAny(tag, "{[\"") {
				rest = rest[newline+1:]
			}
		}
		end := strings.Index(rest, "```")
		if end < 0 {
			return out
		}
		out = append(out, strings.TrimSpace(rest[:end]))
		rest = rest[end+3:]
	}
}

// bracedSpans answers every BALANCED { … } span in text — each a JSON document
// a model may have buried in a sentence.
//
// Balanced, and every one of them, rather than the span from the first brace to
// the last. Prose brackets its own asides: "Here is the answer {as requested}:
// {…} — let me know {if that helps}" has three brace pairs, and first-to-last
// spans all of them plus the words between, which json.Valid then refuses. The
// document was there and readable; the reading was what failed.
//
// Braces inside a string are not structure, so the scan follows JSON's own
// escaping. Without that, a value carrying a brace — a template, a regex, a
// piece of code the model quoted — closes the document early and what is
// offered is a prefix of it.
//
// Balance is where the scan stops and json.Valid takes over: a balanced span is
// a candidate, not a verdict.
func bracedSpans(text string) []string {
	var out []string
	depth, start := 0, -1
	var inString, escaped bool
	for i := 0; i < len(text); i++ {
		switch {
		case escaped:
			escaped = false
		case inString && text[i] == '\\':
			escaped = true
		case text[i] == '"':
			inString = !inString
		case inString:
		case text[i] == '{':
			if depth == 0 {
				start = i
			}
			depth++
		case text[i] == '}':
			if depth == 0 {
				continue
			}
			depth--
			if depth == 0 && start >= 0 {
				out = append(out, text[start:i+1])
				start = -1
			}
		}
	}
	return out
}
