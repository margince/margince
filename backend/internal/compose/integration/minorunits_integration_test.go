// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The database's minor-unit table and Go's must say the same thing, or two
// surfaces convert one deal into two different amounts.
//
// SQL cannot reach values.currencyMinorDigits, so currency_minor_digits is a
// deliberate MIRROR of it — the same arrangement frontend/src/format/
// minorunits.ts has, and held the same way: in BOTH directions, because a code
// missing from either side silently falls back to two digits rather than
// failing, and two digits is the right answer for almost every currency and the
// wrong one for exactly the codes this table exists to name.
//
// It reads the LIVE table rather than the migration text. A gate parsing the
// migration would keep passing after a later migration changed the rows, which
// is the failure a census must not have: it would read a smaller truth, report
// PASS, and leave nothing to notice.

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

func TestTheDatabaseMinorUnitTableMatchesTheGoOne(t *testing.T) {
	// Setup migrates the schema this reads; the rows are installation-wide
	// reference data, so nothing else about the fixture matters here.
	Setup(t)

	rows, err := OwnerConn(t).Query(context.Background(),
		`SELECT currency, digits FROM currency_minor_digits`)
	if err != nil {
		t.Fatalf("reading currency_minor_digits: %v", err)
	}
	defer rows.Close()

	digits := map[string]int{}
	for rows.Next() {
		var code string
		var count int
		if scanErr := rows.Scan(&code, &count); scanErr != nil {
			t.Fatalf("scanning currency_minor_digits: %v", scanErr)
		}
		digits[code] = count
	}
	if rows.Err() != nil {
		t.Fatalf("reading currency_minor_digits: %v", rows.Err())
	}
	if len(digits) == 0 {
		t.Fatal("currency_minor_digits is empty — a comparison against nothing agrees with everything, " +
			"and every foreign conversion in SQL would silently scale at two digits")
	}

	inGo := values.MinorUnitExceptions()
	for code, want := range inGo {
		got, present := digits[code]
		switch {
		case !present:
			t.Errorf("%s is an exception in Go (%d digits) and absent from currency_minor_digits, so "+
				"the rollup view scales it at two and the Go engine at %d — one account, two figures", code, want, want)
		case got != want:
			t.Errorf("%s: Go says %d digits, the database says %d — the same deal converts to two amounts",
				code, want, got)
		}
	}
	for code, got := range digits {
		if _, present := inGo[code]; !present {
			t.Errorf("%s is an exception in currency_minor_digits (%d digits) and absent from Go's table, "+
				"so the Go engine scales it at two", code, got)
		}
	}
}
