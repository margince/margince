// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package accountdraft

// The deterministic anti-AI floor a VOICED draft passes through, after the
// shared correct-and-retry loop has already cleared the phrasing rules.
//
// Its whole job is comparing one draft against another, which is why every
// step here answers the same question: is what I am about to serve better than
// what I already have? A step that cannot answer yes leaves the draft alone.

import (
	"context"
	"strings"

	"github.com/margince/margince/backend/internal/compose/draftcheck"
	"github.com/margince/margince/backend/internal/compose/draftvoice"
	"github.com/margince/margince/backend/internal/modules/ai"
)

// applyVoiceFloor is the deterministic anti-AI pass a voiced draft must clear:
// detect on the raw text, one critic retry that fixes the SENTENCE, then the
// sanitizer for what is left to remove mechanically.
//
// It runs only for a voiced draft. An unvoiced one is already governed by the
// shared rules and by draftcheck, and running a second retry over every draft
// would double the model spend of the common case to fix the rare one.
//
// Every step can only improve the draft, never replace it with a worse one.
// The retry is kept only if it clears more voice violations AND adds no
// phrasing finding — see improves. The sanitizer's edits are kept only if what
// survives them is still a draft: it deletes characters, so a subject that was
// nothing but an em dash comes back empty, and an empty subject is not a
// message the contract allows.
//
// A draft that still trips the floor after all of that is served anyway. The
// alternative is the deterministic floor — a two-line opener — and a rep who
// asked for a draft is better served by an imperfect real message than by a
// stub, which is the same trade Write makes when the model is down.
func applyVoiceFloor(ctx context.Context, lane Completer, in Input, voice draftvoice.Context, draft Draft) (Draft, error) {
	if !voice.OK {
		return draft, nil
	}
	// Detect on the RAW draft: a violation the sanitizer could mechanically
	// remove still earns the retry, because the retry rewrites the sentence
	// where the sanitizer only deletes the punctuation.
	violations := draftvoice.Violations(draft.Subject, draft.Body)
	if len(violations) > 0 {
		retried, retryErr := writeWithModel(ctx, lane, in, voice, draftvoice.Feedback(violations))
		if retryErr == nil && improves(in, draft, retried, violations) {
			draft = retried
		}
	}
	sanitized := draft
	sanitized.Subject, sanitized.Body = draftvoice.Sanitize(draft.Subject, draft.Body)
	if !servable(sanitized) {
		return draft, nil
	}
	return sanitized, nil
}

// improves reports whether the retried draft is better than the one it would
// replace, on BOTH measures.
//
// Two measures, because the retry is answering only one of them. It arrives
// through this package's correct-and-retry loop, so the draft it replaces has
// already cleared the phrasing rules; the retry prompt names the voice
// violations and says nothing about the phrasing findings the first pass fixed.
// A retry judged on voice alone can therefore drop an em dash and invent a
// shared history in the same breath — "we help companies", on a message to
// somebody we have never written to — and score as an improvement while being
// a regression a reader would notice first.
//
// The phrasing test is a SUBSET, not a count. Findings are not
// interchangeable: a false "Re:" claims a message nobody sent us, where a
// wellbeing opener is filler, and counting them equal would let a retry trade
// the second for the first and be served. So the retry may carry only findings
// the draft it replaces already carried.
//
// It is a subset rather than emptiness because the loop serves its better
// attempt rather than a spotless one. Demanding zero findings would discard a
// genuine voice fix over a phrasing finding the incumbent carried too — and the
// incumbent is what we would keep instead, findings and all.
func improves(in Input, current, retried Draft, violations []ai.VoiceViolation) bool {
	if len(draftvoice.Violations(retried.Subject, retried.Body)) >= len(violations) {
		return false
	}
	return noNewPhrasingFindings(phrasingFindings(in, current), phrasingFindings(in, retried))
}

// noNewPhrasingFindings reports whether every finding against the retry was
// already a finding against the draft it would replace.
//
// A multiset: two of the same finding is worse than one, so the incumbent's
// copies are consumed as they are matched. Rule and Phrase together identify
// one — Why is the same sentence for every finding of a rule, so it adds
// nothing to the comparison.
func noNewPhrasingFindings(current, retried []draftcheck.Finding) bool {
	remaining := make(map[draftcheck.Finding]int, len(current))
	for _, finding := range current {
		remaining[identity(finding)]++
	}
	for _, finding := range retried {
		key := identity(finding)
		if remaining[key] == 0 {
			return false
		}
		remaining[key]--
	}
	return true
}

// identity is the part of a finding that says WHICH finding it is.
func identity(finding draftcheck.Finding) draftcheck.Finding {
	return draftcheck.Finding{Rule: finding.Rule, Phrase: finding.Phrase}
}

// servable reports whether a draft is still one the contract can carry. The
// sanitizer removes characters, so a draft made only of what it removes comes
// back empty — and an empty subject or body is not a message.
func servable(draft Draft) bool {
	return strings.TrimSpace(draft.Subject) != "" && strings.TrimSpace(draft.Body) != ""
}
