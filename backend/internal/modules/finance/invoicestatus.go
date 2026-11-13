// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package finance

import "time"

// The mirrored invoice's status vocabulary, matching the column's CHECK and
// the contract's enum. Named rather than repeated: the derivation below and
// the CHECK have to agree, and two spellings of one word is how they stop.
const (
	statusVoid          = "void"
	statusCredited      = "credited"
	statusDisputed      = "disputed"
	statusPaid          = "paid"
	statusOverdue       = "overdue"
	statusPartiallyPaid = "partially_paid"
	statusOpen          = "open"
)

// deriveStatus computes what the invoice is today, from values that do not
// move. Never hashed and never taken from the provider: "overdue" changes at
// midnight, and a stored-and-hashed status would rewrite the ledger every
// morning.
func deriveStatus(inv SourceInvoice, open int64, now time.Time) string {
	switch {
	case inv.Void:
		return statusVoid
	case inv.CreditsExternalID != "":
		return statusCredited
	case inv.FullyPaidAt != nil: // settled outranks a dispute it settled
		return statusPaid
	case inv.Disputed:
		return statusDisputed
	default:
		return settlementStatus(open, inv.PaidMinor, inv.DueOn, now)
	}
}

// settlementStatus is the part of the status that moves with the clock. The
// mirror writes it, and the card derives it again on read, because an
// unchanged source is never rewritten.
func settlementStatus(open, paid int64, due *time.Time, now time.Time) string {
	switch {
	case open == 0:
		// Nothing is owed, whether money, credits or a zero total settled it, so
		// a passed due date makes nothing late.
		return statusPaid
	case pastDue(due, now):
		return statusOverdue
	case paid > 0:
		return statusPartiallyPaid
	default:
		return statusOpen
	}
}

// statusAsOf is a stored status as of now. Void, credited and disputed come
// from the source alone; every other status is settled again against today.
func statusAsOf(stored string, gross, credited, open int64, due *time.Time, now time.Time) string {
	switch stored {
	case statusVoid, statusCredited, statusDisputed:
		return stored
	default:
		return settlementStatus(open, gross-credited-open, due, now)
	}
}
