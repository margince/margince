// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

//go:build !integration

package gates

// Money this product computes has its arithmetic held by the database. Money it
// COPIES does not.
//
// A net/tax/gross triple is derived, not entered: it is summed in Go from the
// lines and written back as three independent values. The lines themselves
// cannot drift — offer_line_item stores no line total — so all the risk sits in
// that one write-back, and a recompute that is missed, partial, or racing
// another leaves three numbers that disagree with each other and with the lines
// they came from. The first reader to notice is the buyer looking at a PDF whose
// total is not its subtotal plus its tax.
//
// The exception is money that arrives already added up. finance_invoice mirrors
// an external finance system under that system's rounding, keyed
// UNIQUE (connection_id, external_id) with sync_hash recording what was copied.
// A CHECK there would refuse to store what the source actually says, turning a
// reconciliation problem into a sync failure — so the rule is about who did the
// arithmetic, not about which columns are present.

import (
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// mirroredMoney are the tables whose triple is COPIED, each saying whose
// arithmetic it is. A table here must still carry the three columns; one that
// stopped would be a stale row rather than an exemption.
var mirroredMoney = gatekit.Waive(map[string]string{
	"finance_invoice": "mirrored from an external finance system — UNIQUE (connection_id, external_id) with " +
		"sync_hash recording what was copied. The numbers are the source's, produced under its rounding, so a " +
		"CHECK refusing them would turn a reconciliation problem into a sync failure",
})

// moneyTriple is the three columns a computed total is made of. The set is the
// subject: a table carrying only net and gross is a different shape and not this
// gate's business.
var moneyTriple = []string{"net_minor", "tax_minor", "gross_minor"}

// sumCheck recognises a CHECK that ties the three together, in either direction
// a reader might write it.
var numericColumn = regexp.MustCompile(`^public\.([a-z0-9_]+)\.([a-z0-9_]+) (?:bigint|integer|numeric)`)

var tableCheck = regexp.MustCompile(`^public\.([a-z0-9_]+)\.[a-z0-9_]+ CHECK `)

var sumCheck = regexp.MustCompile(`gross_minor\s*=\s*\(?\s*net_minor\s*\+\s*tax_minor|net_minor\s*\+\s*tax_minor\s*\)?\s*=\s*gross_minor`)

func TestEveryComputedMoneyTripleAddsUp(t *testing.T) {
	t.Parallel()
	defer mirroredMoney.AssertAllMatched(t)

	columns := map[string]map[string]bool{}
	checks := map[string][]string{}
	for _, record := range catalogRecords(t) {
		record = strings.TrimSpace(record)
		if m := numericColumn.FindStringSubmatch(record); m != nil {
			if columns[m[1]] == nil {
				columns[m[1]] = map[string]bool{}
			}
			columns[m[1]][m[2]] = true
			continue
		}
		if m := tableCheck.FindStringSubmatch(record); m != nil {
			checks[m[1]] = append(checks[m[1]], record)
		}
	}

	carrying := 0
	for table, held := range columns {
		if !carriesAll(held, moneyTriple) {
			continue
		}
		carrying++
		if mirroredMoney.Waived(t, table) {
			continue
		}
		if !anySumCheck(checks[table]) {
			t.Errorf("%s carries net_minor, tax_minor and gross_minor and no CHECK ties them. The three "+
				"are summed in Go and written back independently, so a recompute that is missed or racing "+
				"another leaves them disagreeing with each other and with the lines they came from — and "+
				"nothing refuses the row. Add the CHECK, or register the table as money this product copies "+
				"rather than computes.", table)
		}
	}
	// Under-recognition: a walk that stopped seeing the triple finds nothing to
	// require and reports the same clean schema as one where every total adds up.
	if carrying < len(mirroredMoney.Reasons())+1 {
		t.Fatalf("found %d table(s) carrying the money triple and the register alone names %d — this scan "+
			"has stopped seeing its own subject", carrying, len(mirroredMoney.Reasons()))
	}
}

func carriesAll(held map[string]bool, want []string) bool {
	for _, column := range want {
		if !held[column] {
			return false
		}
	}
	return true
}

func anySumCheck(definitions []string) bool {
	for _, definition := range definitions {
		if sumCheck.MatchString(definition) {
			return true
		}
	}
	return false
}
