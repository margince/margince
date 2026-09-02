// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// What "currently employed" means, in one place. Its own file because it is one
// concept with two readers' worth of reach: the write paths in relationship.go
// decide the flag with it, and four separate READS derive currency with it
// rather than trusting a flag written months earlier.

import "github.com/margince/margince/backend/internal/platform/database/storekit"

// EmploymentIsCurrentSQL is the ONE spelling of "this job is still theirs".
//
// Held by: TestEveryEmploymentCurrencyTestUsesTheOneDefinition (backend/gates/employmentcurrency_test.go)
// — it reads every hand-written Go source outside this file for a hand-spelled
// `ended_at` currency test, so a second definition fails rather than drifting.
// The census reaches its OWN file too: it exempts the two declarations holding
// its planted probes and judges everything else, so a currency test written
// beside them is a finding like any other.
//
// `date` is the
// end-date expression at the call site: a column on a read, the incoming value
// on a create, the patched-or-existing one on an update.
//
// A DATE COMPARISON, not a null check: somebody serving three months' notice
// still works there. Reading the column's mere presence as "gone" took a person
// off their employer's contact list the day their notice was filed, with no way
// back, because `ended_at` cannot be cleared through the API.
//
// `> current_date`, so an employment dated TODAY is already over. That is what
// `ended_at` means in this schema — 0007 documents NULL as "current/ongoing", so
// a date that has arrived is a date that has happened — and it is what keeps the
// rail's "End employment" button doing something the moment it is pressed. A
// future date is the only case that is not yet a departure, which is exactly the
// notice period this predicate exists for.
//
// current_date, evaluated by Postgres. A Go-side comparison would answer a
// different question on a server in a different timezone from the database, and
// every reader of this predicate is SQL that knows only the database's own day.
//
// EXPORTED because currency is not decided once and stored. The flag records
// which employer represents the person; whether that employment is still current
// is a function of today's date, so every READER derives it instead of trusting
// a value written months ago. compose reaches this for the same reason the
// readers in this package do — one definition, or the copies drift.
//
// "One definition" is held by TestEveryEmploymentCurrencyTestUsesTheOneDefinition
// in backend/gates/employmentcurrency_test.go, and it is written down here because
// this comment used to say "the only definition of a current employment in this
// product" with nothing holding it — and it was false eleven times over. Eight
// statements asked with a bare `ended_at IS NULL`, which is the notice-period
// defect described above; three more hand-spelled the correct form; one of
// those compared against a Go clock in the same statement as a half that used
// Postgres', so one query asked its two questions on two different days.
//
// The gate cannot reach five statements in activities, projects and signals: a
// module never imports a sibling (ADR-0054 §3), so those cannot call this at
// all until the predicate moves tier. They are ratified by name in the gate,
// with that reason, rather than left looking clean.
func EmploymentIsCurrentSQL(date string) string {
	return storekit.SQLf("(%s IS NULL OR %s > current_date)", date, date)
}

// CurrentPrimaryEmploymentSQL is what a READER of `is_current_primary` means:
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
func CurrentPrimaryEmploymentSQL(alias string) string {
	prefix := ""
	if alias != "" {
		prefix = alias + "."
	}
	return storekit.SQLf("%sis_current_primary AND %s", prefix, EmploymentIsCurrentSQL(prefix+"ended_at"))
}

// LiveEmploymentSlotSQL is the THIRD question, and the one uq_rel_employment
// answers: does this person already hold a live employment edge to this company
// at all, primary or not.
//
// It is the index's own predicate and so, like CurrentPrimarySlotSQL, it is
// date-BLIND. Asking it with EmploymentIsCurrentSQL would read somebody serving
// notice as having no edge while the index still holds one, and the write that
// followed would be silently dropped by ON CONFLICT rather than skipped — which
// is exactly how a sweep comes to offer the same work on every pass for ever.
//
// `alias` is the relationship table's alias at the call site, or "" when the
// statement does not alias it.
func LiveEmploymentSlotSQL(alias string) string {
	prefix := ""
	if alias != "" {
		prefix = alias + "."
	}
	return storekit.SQLf("%skind = 'employment' AND %sended_at IS NULL AND %sarchived_at IS NULL",
		prefix, prefix, prefix)
}

// CurrentPrimarySlotSQL is the other question about `is_current_primary`:
// WHICH ROW HOLDS THE SLOT that uq_rel_current_primary_employer keeps unique
// per person. It is the index's own predicate, and so it is date-BLIND —
// asking it with EmploymentIsCurrentSQL would read a person serving notice as
// having freed the slot while the index still held it, and the write that
// followed would 409 instead of skipping.
//
// `alias` is the relationship table's alias at the call site, or "" when the
// statement does not alias it.
//
// Held by: TestTheCurrentPrimarySlotPredicateMirrorsItsIndex (backend/internal/modules/people/currentprimaryslot_test.go)
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
	return storekit.SQLf("%skind = 'employment' AND %sis_current_primary AND %sarchived_at IS NULL", prefix, prefix, prefix)
}
