// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// The statutory shield a purge applies, and what it may claim about it.
//
// Its own file because it is the seam's concept rather than the selectors':
// they ask which of a seat's mail a rule reaches, and this carries the rule
// every destructive path in the installation is held to.

// StatutoryFloor carries the shield every destructive activity path in this
// installation applies, handed in by the compose seam.
//
// Capture cannot import privacy, and the predicate lives over there — a second
// copy is how one destructive path quietly stops shielding what the others do.
// So it travels: the seam reads it from privacy and passes it here.
//
// Held by: TestTheStatutoryFloorIsSpelledOnce (backend/gates/statutoryfloorsingle_test.go).
type StatutoryFloor struct {
	// Clause filters an activity aliased `a`, in the positive form: it is TRUE
	// for a row the law still requires the installation to keep.
	Clause func(intervalArg, anchorArg int) string
	// Interval is the retention period, as a SQL interval literal; Anchor says
	// whether the window runs from the end of the calendar year.
	Interval string
	Anchor   bool
}

// column renders the shield as a boolean expression and appends its two
// arguments, returning where they landed.
func (f StatutoryFloor) column(used int, args []any) (string, []any) {
	if f.Clause == nil {
		// No floor supplied. Shield EVERYTHING rather than nothing: a purge
		// that cannot ask what the law requires must not guess that the answer
		// is "nothing", because that guess destroys correspondence.
		//
		// What it must ALSO not do is report the result as a statutory
		// window — see determinate.
		return "true", args
	}
	return f.Clause(used+1, used+2), append(args, f.Interval, f.Anchor)
}

// shieldedAs is the reason a row shielded by this floor is reported under.
//
// An absent clause shields every row, which is the safe answer to "may this be
// destroyed". It is not an answer to "why was it kept", and the two were the
// same value: every shielded row came back labelled `statute`, so an owner was
// told their mail is commercial correspondence the law requires keeping when
// nothing had established that. The shield stays; the reason it reports does
// not claim a basis nobody determined.
func (f StatutoryFloor) shieldedAs() string {
	if f.Clause == nil {
		return withheldByUndeterminedFloor
	}
	return withheldByStatute
}
