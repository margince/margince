// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/mailsubject"
)

// The answer check strips a subject in SQL, and certification fixtures are
// checked in Go. Both must read one subject the same way, or a fixture is
// certified against mail the evidence read would never offer.
func TestNormalizedMatchesTheAnswerCheck(t *testing.T) {
	e := setupLoad(t)
	for _, subject := range []string{
		"Invoice", "Re: Invoice", "AW: RE:  Invoice", "antw:Invoice", "Fwd: Invoice",
		"WG: Re: Invoice", "  Signed   NDA  ", "RE:re: Unterlagen", "", "Rechnung Re: Invoice",
		"Re:\u2003Signed\u2003NDA", "\u00a0Invoice\u00a0", "Re:\u202fNDA\u2007", "\tRe:\nNDA ",
	} {
		var sql string
		if err := e.owner.QueryRow(e.as(), `SELECT `+normalisedSubject("$1::text"), subject).Scan(&sql); err != nil {
			t.Fatalf("normalising %q in SQL: %v", subject, err)
		}
		if got := mailsubject.Normalized(subject); got != sql {
			t.Errorf("%q normalises to %q in Go and %q in the answer check", subject, got, sql)
		}
	}
}
