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
	p := ""
	if alias != "" {
		p = alias + "."
	}
	return fmt.Sprintf(`(
		%[1]sarchived_at IS NULL
		AND %[1]sstatus NOT IN ('draft', 'superseded')
		AND (%[1]sstarts_on IS NULL OR %[1]sstarts_on <= %[2]s)
		AND (LEAST(%[1]sends_on, %[1]scancellation_effective_on) IS NULL
		     OR %[2]s <= LEAST(%[1]sends_on, %[1]scancellation_effective_on))
	)`, p, asOf)
}
