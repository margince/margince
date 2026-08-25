// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package idlebase is the one spelling of "when did this record last show a
// sign of life".
//
// Held by: TestTheIdleBaseIsSpelledOnce (backend/oneidlebase_test.go)
//
// Deal, project, organization and person all carry the same pair: a
// last_activity_at maintained from the timeline, and the created_at that is
// always there. Every surface that measures silence — the stalled-deal rule,
// the gone-quiet project report, an account's coverage view, the ranked
// what's-slipping set — falls back from the first to the second, and five of
// them derived that fallback for themselves. Two carried a comment CLAIMING
// to match the stalled rule, which is the shape this repository's rulebook
// forbids: a comment may not claim to be the only implementation unless a test
// holds it. Nothing held either.
//
// The fallback is not a formatting detail. Reading an untouched record as "no
// data" would hide exactly the records every one of those surfaces exists to
// find: the ones nobody has spoken to since the day they were written down.
// So the two spellings are here together — the Go one a caller applies to
// scanned values, the SQL one a caller embeds in an ORDER BY, a WHERE or a
// projection — rather than in the module that first needed them.
//
// Stdlib-only and in kernel because the callers are three different modules
// plus two compose subpackages, and a module never imports a sibling (ADR-0054
// §3). Putting it in deals, which is where it was first spelled, would have
// left projects reaching across that line.
package idlebase

import (
	"fmt"
	"time"
)

// Since is the idle base of one record: its newest recorded activity when
// there is one, else the instant it was created.
func Since(createdAt time.Time, lastActivityAt *time.Time) time.Time {
	if lastActivityAt != nil {
		return *lastActivityAt
	}
	return createdAt
}

// SQL is the same fallback as an SQL expression over the query's table alias.
//
// alias is the bare alias a caller gave the row's table, "" for an unaliased
// query — the dot is added here, so no caller has to remember whether it
// belongs. A query that JOINs a second table carrying either column MUST name
// an alias: unqualified, the reference is ambiguous SQL rather than merely
// pointing at the wrong row.
func SQL(alias string) string {
	prefix := ""
	if alias != "" {
		prefix = alias + "."
	}
	return fmt.Sprintf("coalesce(%[1]slast_activity_at, %[1]screated_at)", prefix)
}
