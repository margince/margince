// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package draftcore holds what must not differ between drafting surfaces.
//
// There are three: the reply to an activity, the contact composer, and
// account-started outbound. They differ in what a draft is grounded IN — an
// activity, a Contact360, a Company360 — and that difference is real and
// stays with each surface. What they share is the machinery around generation,
// and every time a copy of it drifted, a defect shipped on one surface and not
// the others.
//
// This package owns the correct-and-retry loop, which is the piece with the
// most judgement in it. The surface supplies two closures — write once, read
// what came back — and gets back the draft to serve. It holds no prompt text,
// no schema and no grounding: those are per-surface by design, and a package
// that owned them would be a fourth surface pretending to be a library.
//
// It lives in the composition layer because its consumers are compose
// subpackages that may not import each other.
package draftcore

import (
	"context"

	"github.com/margince/margince/backend/internal/compose/draftcheck"
	"github.com/margince/margince/backend/internal/shared/kernel/convstate"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// Writer produces one draft. correction is empty on the first attempt and
// carries the feedback naming what was wrong on the retry; the surface decides
// where in its own prompt that text belongs.
type Writer[D any] func(ctx context.Context, correction string) (D, error)

// Observer is told what the loop decided, so a surface keeps the operational
// visibility the loop takes over. Optional: nil observes nothing.
//
// It exists because a retry that does not help is invisible from the outside —
// the caller gets a draft either way — and "the model kept producing rejected
// phrasing" is exactly the signal that says a phrase list or a prompt rule
// needs work.
type Observer interface {
	// RetryFailed: the correction call itself errored, and the first draft
	// stands.
	RetryFailed(ctx context.Context, findings int, err error)
	// RetryDidNotClear: the retry answered, and something survived it.
	//
	// The RULE rather than its name, because "the retry did not clear" means
	// something different for a false claim than for a phrasing tic, and an
	// operator reading the line needs to be able to tell which without knowing
	// every rule by heart. `remaining` counts the DISTINCT rules still broken,
	// which is the number the serve decision was made on.
	RetryDidNotClear(ctx context.Context, rule draftcheck.Rule, phrase string, remaining int)
}

// TextOf reads the two channels of a draft a check has to judge.
//
// Both, because they fail differently and only one used to be read. The BODY is
// what the recipient sees. The REASONING labels are what the product tells the
// rep it wrote from, and a rep reads a chip less critically than the sentence
// they are about to send — so a wrong chip is the worse of the two. An invented
// "introduction by Romina Medici" reached one on a real thread while the body
// beside it was correct.
//
// The subject is deliberately absent: it is one line with its own rules, and
// folding it in would let a phrase banned in prose hide there.
type TextOf[D any] func(D) (body string, reasoning []string)

// SubjectOf reads a draft's subject line and whether a real inbound thread
// earns it a reply prefix. Separate from TextOf because a subject fails
// differently — its worst failure is a CLAIM ("Re:" says a thread exists)
// rather than a phrase — and because a surface with no subject supplies none.
type SubjectOf[D any] func(D) (subject string, threaded bool)

// CorrectOnce writes a draft, checks it against the correspondence it was
// written into, and gives the model exactly one chance to fix what it got wrong.
//
// One retry, because the alternatives are both worse. Zero leaves a defect the
// product knows about in text a human is about to send. Two or more is a model
// that will not comply, paid for twice more, while a deterministic floor sits
// underneath that was always going to be the answer.
//
// The correction names the exact phrase back to the model, which is the thing a
// prompt sentence cannot do — and the reason this loop exists at all is that
// three separate prompt rules lost to model reflexes before it did.
//
// When the retry does not clear the findings, the attempt carrying FEWER of them
// is served. A second attempt is not automatically better, and the count is the
// only evidence available without asking a model to judge its own output.
// Findings is everything the phrasing rules say is wrong with one draft.
//
// Exported because CorrectOnce is not the only pass that has to ask. The voice
// floor runs AFTER this loop and may substitute a retried draft, so it needs
// the same answer to know whether the substitution regressed anything — and a
// second spelling of "what is wrong with a draft" would drift from this one
// until the two disagreed about a phrase in front of a user.
func Findings[D any](
	draft D, lang textlang.Lang, band convstate.Band, booked bool,
	textOf TextOf[D], subjectOf SubjectOf[D],
) []draftcheck.Finding {
	body, reasoning := textOf(draft)
	// Whether this draft answers a real inbound message decides more than the
	// subject's reply prefix: a reply is written from the counterparty's own
	// words, so it may echo a call THEY named, where a message opening a new
	// conversation has no such ground to stand on.
	subject, threaded := "", false
	if subjectOf != nil {
		subject, threaded = subjectOf(draft)
	}
	findings := append(draftcheck.Body(body, lang, band, draftcheck.Grounds{Threaded: threaded, Booked: booked}),
		draftcheck.Reasoning(reasoning, lang, band)...)
	// Shape is asked of the BODY alone. Reasoning chips travel through the same
	// phrasing rules but are labels, not messages.
	findings = append(findings, draftcheck.Formatting(body)...)
	if subjectOf != nil {
		findings = append(findings, draftcheck.Subject(subject, lang, band, threaded)...)
	}
	return findings
}

func CorrectOnce[D any](
	ctx context.Context, lang textlang.Lang, band convstate.Band, booked bool,
	write Writer[D], textOf TextOf[D], subjectOf SubjectOf[D], observe Observer,
) (D, error) {
	check := func(draft D) []draftcheck.Finding {
		return Findings(draft, lang, band, booked, textOf, subjectOf)
	}

	draft, err := write(ctx, "")
	if err != nil {
		var zero D
		return zero, err
	}

	findings := check(draft)
	if len(findings) == 0 {
		return draft, nil
	}

	retried, retryErr := write(ctx, draftcheck.Feedback(findings))
	if retryErr != nil {
		// The first attempt stands. It carries the defect and it is still a
		// real message a human can edit, which beats refusing to answer.
		if observe != nil {
			observe.RetryFailed(ctx, len(findings), retryErr)
		}
		return draft, nil
	}

	remaining := check(retried)
	if len(remaining) == 0 {
		return retried, nil
	}
	if observe != nil {
		observe.RetryDidNotClear(ctx, remaining[0].Rule, remaining[0].Phrase, draftcheck.Rules(remaining))
	}
	if servesRetry(findings, remaining) {
		return retried, nil
	}
	return draft, nil
}

// servesRetry decides which attempt a reader would call safer.
//
// SEVERITY FIRST, then the count of distinct rules, and neither half alone is
// the answer.
//
// The raw finding count was not comparable across rules, because the rules do
// not match at the same rate: the band-gated loops append one finding per
// matching phrase while the world-claim rules report one per rule. So a draft
// saying the same wrong thing three ways scored 3 and a draft making two
// separate false statements scored 2, and the more serious one was discarded.
//
// Deduplicating by rule makes the two styles comparable and still gets that
// case wrong: three phrasings of one memory assumption is ONE rule against the
// other draft's TWO, so a false statement about the world still loses to a
// phrasing habit. Severity is what fixes it, and it is the half a reader would
// have used first: a rep sends what the product wrote, and an invented call
// reaches the recipient as the company's own word.
//
// A TIE goes to the retry, unchanged. Both attempts carry one finding often
// enough to matter — the model swaps "circling back" for "checking in" — and
// the retried one was at least written with the correction in hand, so it is
// the better bet on everything the check does not measure. Only a retry that is
// strictly worse is discarded.
func servesRetry(first, retried []draftcheck.Finding) bool {
	firstWorst, _ := draftcheck.Worst(first)
	retriedWorst, _ := draftcheck.Worst(retried)
	if retriedWorst != firstWorst {
		return retriedWorst < firstWorst
	}
	if firstRules, retriedRules := draftcheck.Rules(first), draftcheck.Rules(retried); retriedRules != firstRules {
		return retriedRules < firstRules
	}
	// SAME SEVERITY AND THE SAME NUMBER OF RULES: the raw match count is the
	// last thing that separates them, and here it is honest. The comparison the
	// ticket objected to was across DIFFERENT rules, where one rule reports per
	// phrase and another reports once however many matched; by this point both
	// drafts break the same number of rules, and a draft saying the same wrong
	// thing three ways is more of it than a draft saying it once.
	return len(retried) <= len(first)
}
