// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package contractterm is what "this contract is in force on a day" means
// (CONTRACT-FORM-1), in one place every module can reach. The contracts module
// lists a company's agreements with it, and the send rules ask it whether a
// contract notice has a live agreement behind it.
//
// stdlib only, which the kernel tier requires.
package contractterm

import "fmt"

// InForceSQL is true when the contract row aliased alias is in force on the
// day asOf, an SQL expression. A draft and a superseded contract are never in
// force. The effective end is the earlier of the term end and the cancellation
// effective date. `LEAST` ignores NULLs, so a contract with neither is
// open-ended. A cancellation never extends a term that already ran out. An
// empty alias names the columns bare.
func InForceSQL(alias, asOf string) string {
	p := prefix(alias)
	return fmt.Sprintf(`(
		%[1]sarchived_at IS NULL
		AND %[1]sstatus NOT IN ('draft', 'superseded')
		AND %[2]s
		AND %[3]s
	)`, p, startedSQL(p, asOf), notEndedSQL(p, asOf))
}

// The term states TermStateSQL yields, spelled as the API spells them.
const (
	TermNotStarted = "not_started"
	TermInForce    = "in_force"
	TermEnded      = "ended"
)

// TermStateSQL is where the day asOf falls in the contract's dated term, as an
// SQL expression yielding one of the Term* states. It reads the dates alone.
// InForceSQL adds the status and archival to the same two date tests.
func TermStateSQL(alias, asOf string) string {
	p := prefix(alias)
	return fmt.Sprintf(`(CASE
		WHEN NOT %[1]s THEN '%[3]s'
		WHEN NOT %[2]s THEN '%[4]s'
		ELSE '%[5]s'
	END)`, startedSQL(p, asOf), notEndedSQL(p, asOf), TermNotStarted, TermEnded, TermInForce)
}

func startedSQL(p, asOf string) string {
	return fmt.Sprintf(`(%[1]sstarts_on IS NULL OR %[1]sstarts_on <= %[2]s)`, p, asOf)
}

func notEndedSQL(p, asOf string) string {
	return fmt.Sprintf(`(LEAST(%[1]sends_on, %[1]scancellation_effective_on) IS NULL
		     OR %[2]s <= LEAST(%[1]sends_on, %[1]scancellation_effective_on))`, p, asOf)
}

func prefix(alias string) string {
	if alias == "" {
		return ""
	}
	return alias + "."
}
