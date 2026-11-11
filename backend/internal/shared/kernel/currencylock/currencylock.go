// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package currencylock is why the base currency stopped being changeable: facts
// a client words in its reader's language, and English prose for older ones.
package currencylock

import (
	"fmt"
	"strings"
)

// Reason names which of the two things holds the base currency.
type Reason string

const (
	// RatesFrozen means records have frozen a conversion rate against the base,
	// and nothing undoes that.
	RatesFrozen Reason = "rates_frozen"
	// RateSheet means the rate sheet converts into the base. Clearing it lifts
	// the lock.
	RateSheet Reason = "rate_sheet"
)

// Lock is one answer to "why can the base currency not change".
type Lock struct {
	Reason Reason
	Count  int
	// Frozen is what Count was counted from, one entry per table that holds a
	// frozen rate, in the order the probe counted them.
	Frozen []Counted
	// BaseCurrency is the ISO code the sheet converts into, or "" when the
	// setting row is absent and the base cannot be named.
	BaseCurrency string
}

// Counted is how many rows of one table froze a rate.
type Counted struct {
	Table string
	Count int
}

// frozenKinds names, in words, the records each counted table holds.
var frozenKinds = map[string]string{
	"deal":             "closed deals",
	"offer":            "sent offers",
	"finance_invoice":  "mirrored invoices",
	"contract":         "contracts",
	"commission_entry": "commission entries",
}

// FrozenKind answers what a counted table holds in words, and whether this
// package has words for it.
func FrozenKind(table string) (string, bool) {
	kind, named := frozenKinds[table]
	return kind, named
}

// Prose is the English reason older clients render as it stands. A reason
// this package does not name, the zero Lock included, has no prose.
func (l Lock) Prose() string {
	switch l.Reason {
	case RatesFrozen:
		return fmt.Sprintf("%d record(s)%s have already frozen a conversion rate against it, so changing "+
			"the base would re-mean every roll-up built on them", l.Count, l.frozenBreakdown())
	case RateSheet:
		base := l.BaseCurrency
		if base == "" {
			base = "the base it was set to"
		}
		return fmt.Sprintf(
			"%d rate(s) in the sheet convert into %s and cannot be restated against another base; "+
				"clear the rate sheet first", l.Count, base)
	default:
		return ""
	}
}

// frozenBreakdown names the records that froze a rate and how many of each,
// as " (contracts: 2)", or nothing when the probe counted none by table.
func (l Lock) frozenBreakdown() string {
	parts := make([]string, 0, len(l.Frozen))
	for _, counted := range l.Frozen {
		if counted.Count == 0 {
			continue
		}
		kind, named := FrozenKind(counted.Table)
		if !named {
			kind = counted.Table
		}
		parts = append(parts, fmt.Sprintf("%s: %d", kind, counted.Count))
	}
	if len(parts) == 0 {
		return ""
	}
	return " (" + strings.Join(parts, ", ") + ")"
}

// Refusal is the reason a refused write carries. A lock with no prose still
// refuses, and an empty reason would tell the operator nothing.
func (l Lock) Refusal() string {
	if prose := l.Prose(); prose != "" {
		return prose
	}
	return "conversion rates are already frozen against the base currency, so it cannot change"
}
