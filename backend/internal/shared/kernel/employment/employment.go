// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package employment is what "this job is still theirs" means, in one place
// every module can reach.
//
// It sat in `modules/contacts` and answered for that module only. A module never
// imports a sibling, so eight statements in five other modules — activities,
// projects, signals, consent and search — hand-spelled the question instead,
// and each hand-spelling was the notice-period defect the helper exists to
// stop. The gate that holds "one definition" had to ratify all eight by name.
//
// Tier 0 rather than storekit, which was the other candidate. storekit is
// documented as owning no domain and "is this employment current" is a domain
// rule, so putting it there would have bought reach by bending a stated
// boundary. shared/kernel already holds domain rules of exactly this kind —
// values' money scales, elapsed's calendar-day counting, draftfloor's copy —
// so this joins a category rather than opening one.
//
// stdlib only, which the tier requires. The one thing it took from storekit
// was SQLf, and SQLf is fmt.Sprintf under a name that says the string is SQL;
// that name is worth keeping and the dependency is not.
//
// One concept with two kinds of reach: the write paths in contacts/relationship.go
// decide the flag with it, and the reads derive currency with it rather than
// trusting a flag written months earlier.
package employment

import (
	"fmt"
	"regexp"
	"strings"
)

var employmentEndColumn = regexp.MustCompile(`^(?:[A-Za-z_][A-Za-z_0-9]*\.)?ended_at$`)

// IsCurrentSQL treats a future departure as a notice period: the contact still works there.
// A departure day that has arrived is already over, so ending employment today
// removes it from current-employer views immediately. Postgres owns the clock
// because every server-side reader evaluates this rule in its transaction.
// Expression callers provide status and precision explicitly; only a plain
// ended_at column (optionally qualified) derives its sibling columns.
// IsCurrentSQL combines assertion and date evidence. A former or unknown
// employment never becomes current merely because its end date is missing.
// Legacy rows without an assertion retain the date-based rule. Month precision
// holds through the whole month; an exact departure day is already departed.
// Column expressions derive their sibling status/precision columns; write
// expressions supply those explicitly. Fragments are trusted source SQL.
// Held by TestEveryEmploymentCurrencyTestUsesTheOneDefinition.
func IsCurrentSQL(date string, asserted ...string) string {
	status, precision := "NULL", "NULL"
	if employmentEndColumn.MatchString(date) {
		prefix := strings.TrimSuffix(date, "ended_at")
		status, precision = prefix+"employment_status", prefix+"ended_precision"
	}
	if len(asserted) > 0 {
		status = asserted[0]
	}
	if len(asserted) > 1 {
		precision = asserted[1]
	}
	end := sqlf("CASE WHEN %s = 'month' THEN (date_trunc('month', %s::date) + interval '1 month')::date ELSE %s END", precision, date, date)
	return sqlf("(coalesce(%s, 'current') = 'current' AND (%s IS NULL OR %s > current_date))", status, date, end)
}

// CurrentPrimarySQL is what a READER of `is_current_primary` means:
// the flag AND the employment still being theirs. Spelled once so a new reader
// cannot trust the flag alone, which is what let somebody go on counting at a
// company after their last day had passed.
//
// Held by: TestEveryEmploymentCurrencyTestUsesTheOneDefinition (backend/gates/employmentcurrency_test.go)
// — the same census: a reader that pairs the flag with its own date test is a
// second definition and fails there.
//
// READERS, not every mention of the column. The uniqueness guards must stay
// date-BLIND and deliberately do — that is the OTHER question about this
// column, and CurrentPrimarySlotSQL below is its one spelling. A guard that
// used this helper would think the slot was free while the index still held
// it, and answer 409 instead of skipping. Two different questions about one
// column; this one is "who works there now".
func CurrentPrimarySQL(alias string) string {
	prefix := ""
	if alias != "" {
		prefix = alias + "."
	}
	return sqlf("%sis_current_primary AND %s", prefix, IsCurrentSQL(prefix+"ended_at"))
}

// LiveSlotSQL is the THIRD question, and the one uq_rel_employment
// answers: does this contact already hold a live employment edge to this company
// at all, primary or not.
//
// It is the index's own predicate and so, like CurrentPrimarySlotSQL, it is
// date-BLIND. Asking it with IsCurrentSQL would read somebody serving
// notice as having no edge while the index still holds one, and the write that
// followed would be silently dropped by ON CONFLICT rather than skipped — which
// is exactly how a sweep comes to offer the same work on every pass for ever.
//
// `alias` is the relationship table's alias at the call site, or "" when the
// statement does not alias it.
func LiveSlotSQL(alias string) string {
	prefix := ""
	if alias != "" {
		prefix = alias + "."
	}
	return sqlf("%skind = 'employment' AND %sended_at IS NULL AND %sarchived_at IS NULL AND coalesce(%semployment_status, 'current') = 'current'",
		prefix, prefix, prefix, prefix)
}

// CurrentPrimarySlotSQL is the other question about `is_current_primary`:
// WHICH ROW HOLDS THE SLOT that uq_rel_current_primary_employer keeps unique
// per contact. It is the index's own predicate, and so it is date-BLIND —
// asking it with IsCurrentSQL would read a contact serving notice as
// having freed the slot while the index still held it, and the write that
// followed would 409 instead of skipping.
//
// `alias` is the relationship table's alias at the call site, or "" when the
// statement does not alias it.
//
// Held by: TestTheCurrentPrimarySlotPredicateMirrorsItsIndex (backend/internal/modules/contacts/currentprimaryslot_test.go)
// — it derives the expectation from uq_rel_current_primary_employer in the
// migration head catalog, so this cannot drift from the index it exists to
// satisfy.
//
// Two gates and not one, because either alone reads green over a second
// spelling: TestEveryCurrentPrimarySlotGuardUsesTheOneSpelling in
// backend/gates/employmentcurrency_test.go reads every hand-written Go source for the
// FRAGMENT — the flag sharing a conjunction with an archived test — which a
// census that knows only the whole predicate cannot see.
func CurrentPrimarySlotSQL(alias string) string {
	prefix := ""
	if alias != "" {
		prefix = alias + "."
	}
	return sqlf("%skind = 'employment' AND %sis_current_primary AND %sarchived_at IS NULL", prefix, prefix, prefix)
}

// sqlf renders a SQL fragment. Named rather than calling fmt.Sprintf inline,
// for the reason storekit.SQLf is: a formatted string that reaches a database
// should say so at the call site, so a reader checks it for what a formatted
// SQL string is checked for.
//
// Only identifiers and other fragments are ever formatted in here — never a
// value, which is what the placeholders are for.
func sqlf(format string, a ...any) string { return fmt.Sprintf(format, a...) }
