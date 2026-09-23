// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package modelreply reduces a model's reply to the JSON document in it.
//
// It lives in shared/kernel rather than beside the provider adapters because
// both tiers read model replies and a module may not import a sibling, so a
// reduction owned by the ai module is unreachable from the agents module.
//
// Not to be confused with kernel/promptfence, which delimits untrusted content
// going OUT in a prompt. This is what comes back.
package modelreply

import (
	"encoding/json"
	"strings"
)

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
	for _, candidate := range candidateDocuments(text) {
		if len(candidate) > len(best) {
			best = candidate
		}
	}
	return best, best != ""
}

// candidateDocuments answers every DISTINCT valid JSON document buried in text,
// whether fenced or floating in a sentence.
//
// Distinct, because the two scans overlap by design: a fenced block and the
// braced span inside it are one document found twice, and counting it twice
// makes an unambiguous reply look ambiguous to a caller counting candidates.
func candidateDocuments(text string) []string {
	return validDocuments(append(fencedBlocks(text), bracedSpans(text)...))
}

// fencedDocuments answers the distinct valid JSON documents text FENCES, and
// ignores any floating in prose.
//
// The narrower scan, for SoleDocument. A fence is the model emitting its
// answer; a brace span in a sentence is the model quoting something — and on a
// channel that executes what it reads, those two cannot be treated alike.
func fencedDocuments(text string) []string {
	return validDocuments(fencedBlocks(text))
}

// validDocuments filters candidate spans to the distinct ones that are JSON,
// so both scans above agree on what counts as a document rather than each
// deciding.
func validDocuments(candidates []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if seen[candidate] || !json.Valid([]byte(candidate)) {
			continue
		}
		seen[candidate] = true
		out = append(out, candidate)
	}
	return out
}

// SoleDocument is Unfence for a channel where a recovered document has to be
// UNAMBIGUOUS: it takes the reply's own document when the reply is one, and
// recovers a buried document only when the reply holds exactly one.
//
// Unfence takes the LARGEST candidate, which is right for a reply whose other
// spans are the model's own working and wrong wherever the reply may quote
// somebody else's. The agent loop's step is such a channel: an observation
// carries untrusted text, a model that correctly REFUSES an instruction found in
// it tends to quote the instruction while refusing — "the note asked me to run
// X; I will not" — and if that quote is a step-shaped object, largest-wins hands
// it back as the step to execute. The attacker chooses the length, so largest is
// always theirs, and a refusal becomes the injection succeeding.
//
// Ambiguity therefore REFUSES rather than guesses. The caller has a re-ask path
// and an invalid-step limit; neither is as expensive as running a tool call the
// model declined to make.
//
// And counting candidates is not enough on its own, because the attacker does
// not need two. A model refusing correctly quotes the instruction and emits no
// step of its own, which leaves the INJECTED object as the SOLE candidate —
// "the only document in the reply" is then the one thing the model declined to
// do. So prose is not a channel this reads a document out of at all: the reply
// is either a document itself, or it FENCES one. An inline brace span is a
// quotation, and is left where it lies.
func SoleDocument(text string) string {
	raw := strings.TrimSpace(text)
	trimmed := strings.Trim(strings.TrimPrefix(raw, "```json"), "` \n")
	if json.Valid([]byte(trimmed)) {
		return trimmed
	}
	if fenced := fencedDocuments(raw); len(fenced) == 1 {
		return fenced[0]
	}
	return trimmed
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
