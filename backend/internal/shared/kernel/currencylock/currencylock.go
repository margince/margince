// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package currencylock is why the base currency stopped being changeable: facts
// a client words in its reader's language, and English prose for older ones.
package currencylock

import "fmt"

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
	// BaseCurrency is the ISO code the sheet converts into, or "" when the
	// setting row is absent and the base cannot be named.
	BaseCurrency string
}

// Prose is the English reason older clients render as it stands. A reason
// this package does not name, the zero Lock included, has no prose.
func (l Lock) Prose() string {
	switch l.Reason {
	case RatesFrozen:
		return fmt.Sprintf(
			"%d record(s) — closed deals, sent offers, mirrored invoices, contracts and commission entries — "+
				"have already frozen a conversion rate against it, so changing the base would re-mean "+
				"every roll-up built on them", l.Count)
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
