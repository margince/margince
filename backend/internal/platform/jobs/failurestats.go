// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package jobs

// How many jobs are failing, and of WHAT.
//
// The runtime gauges next door answer how much work is stuck; this answers why,
// in the vocabulary an operator already filters by. The difference is what makes
// an outage alertable rather than merely readable: until now the only signal a
// monitor could see was the discarded count going up, and that fires identically
// for a provider outage, a revoked credential and a bug. Those three want three
// different responses.
//
// The class is not stored. A failure row holds a vetted SENTENCE and nothing
// else — no class prefix, no envelope — so the class is derived on read from the
// row's kind and its text, exactly as the failure list derives it. This read
// therefore returns the stored text and lets its caller classify, rather than
// putting a second classifier in SQL that could disagree with the first.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

// FailureCount is one (kind, stored sentence) pair and how many jobs are
// sitting on it.
type FailureCount struct {
	Kind string
	// Stored is river_job.errors' newest message VERBATIM, which is what the
	// class is resolved from. Empty when the row records no cause at all —
	// a state its own, and not the same as a cause nobody has classified.
	Stored string
	Count  int64
}

// failureStates are the states this count is ABOUT: work that failed and is
// either waiting to try again or has given up.
//
// `cancelled` is deliberately not among them. A cancellation is a deliberate
// stop — the exposition already counts those apart for exactly that reason —
// and an alert that could not tell an operator's own decision from a provider
// outage is the confusion this metric exists to end, not one to introduce a
// second time.
//
// `retryable` IS among them, and that is the half that makes the metric worth
// having: a connector unreachable all night spends the night retrying, and an
// alert that waited for every attempt to be spent would fire the morning after
// the outage it was supposed to catch.
const failureStates = `('retryable','discarded')`

// statsByFailure groups the failing rows by kind and by the sentence they
// recorded.
//
// Grouped in SQL rather than counted in Go over the failure LIST, because that
// list is capped at the newest few hundred rows for a screen: a metric read off
// it would under-report exactly when a fleet is failing most, which is the one
// time it is being watched.
func statsByFailure(ctx context.Context, pool *pgxpool.Pool) ([]FailureCount, error) {
	const q = `
		SELECT kind,
		       coalesce(errors[cardinality(errors)]->>'error', ''),
		       count(*)::bigint
		FROM river_job
		WHERE state::text IN ` + failureStates + `
		GROUP BY 1, 2`

	cursor, err := pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("jobs: reading job failures by class: %w", err)
	}
	defer cursor.Close()

	var out []FailureCount
	for cursor.Next() {
		var f FailureCount
		if err := cursor.Scan(&f.Kind, &f.Stored, &f.Count); err != nil {
			return nil, fmt.Errorf("jobs: scanning job failures by class: %w", err)
		}
		out = append(out, f)
	}
	if err := cursor.Err(); err != nil {
		return nil, fmt.Errorf("jobs: reading job failures by class: %w", err)
	}
	return out, nil
}

// CoreFailureClasses answers the core vocabulary's class tokens.
//
// Exported for the exposition's series bound: the number of series this metric
// can publish is (core classes + each unit's own + one reserved) × the kinds
// that fail, and a bound nobody can count is a bound nobody is keeping.
func CoreFailureClasses() []string {
	out := make([]string, 0, len(vocabulary))
	for _, known := range vocabulary {
		out = append(out, known.class)
	}
	return out
}

// SentenceForClass answers the sentence a core class is recorded as, or the
// empty string for a token the core vocabulary does not own.
//
// Exported for the metric's own cases: a test that typed the sentence out
// would keep passing while the wording it copied changed underneath, and the
// classification it is exercising is a lookup FROM that wording.
func SentenceForClass(class string) string {
	for _, known := range vocabulary {
		if known.class == class {
			return known.sentence
		}
	}
	return ""
}
