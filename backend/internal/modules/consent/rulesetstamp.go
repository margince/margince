// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// Naming the rules a decision was judged under, on the decision itself.
//
// messaging.Rules has said since it shipped that its Version is "stamped onto
// every decision taken under it". Nothing stamped it, and
// gates/messagingruleapplied_test.go carried the gap in its register. This is
// the reader that closes it.
//
// The promise is to a SUBJECT, not to an operator: somebody asking a year later
// which rules judged their message is owed an answer from the record, and
// re-deriving it from whatever the packs say today would answer a different
// question.

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// rulesetStamp is what a decision records about the rules that judged it.
//
// A VALUE rather than two returns, because the pair travels together from the
// one lookup to the insert. Splitting them let a caller read the rules a second
// time between deciding and recording, which is how a row could name a pack
// that judged nothing.
type rulesetStampValue struct {
	// Version is the pack's own, non-nil only when exactly one jurisdiction
	// applied. A fold carries none.
	Version *int
	// Codes are the jurisdictions that contributed, and the answer in both
	// cases.
	//
	// NIL when no country is declared, never an empty slice: pgx writes nil as
	// NULL, and the CHECK in migration 1789240000 refuses an empty array so
	// that "no jurisdiction applied" has exactly one spelling in the record.
	Codes []string
}

// rulesetStamp answers what to record about the rules that judged this message.
//
// TWO ANSWERS, because one cannot carry it. A single jurisdiction's rule set
// carries its own version. A fold of two carries none — messagingrules.stricter
// zeroes it deliberately, because stamping a fold with one country's number
// would misname the rules the decision was actually taken under. The codes
// answer in both cases, so they are what a reader keys on and the version is the
// refinement available when exactly one jurisdiction applied.
//
// BOTH NULL is a real answer. An installation that declares no country resolves
// to no rules at all, and that is recorded as absence rather than as zero: a
// zero version would read as a ruleset that exists and has no version.
func (g *Gate) rulesetStamp(ctx context.Context, tx pgx.Tx) (rulesetStampValue, error) {
	rules, applied, found, err := g.store.applicableRules(ctx, tx)
	if err != nil {
		return rulesetStampValue{}, err
	}
	if !found || len(applied.Codes) == 0 {
		return rulesetStampValue{}, nil
	}
	codes := make([]string, 0, len(applied.Codes))
	for _, code := range applied.Codes {
		codes = append(codes, string(code))
	}
	// The version travels only with a single jurisdiction, which is the same
	// condition Applied.Folded reports and the CHECK in migration 1789240000
	// enforces from the other side.
	if applied.Folded() {
		return rulesetStampValue{Codes: codes}, nil
	}
	version := rules.Version
	return rulesetStampValue{Version: &version, Codes: codes}, nil
}
