// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package finance

// The mirror's own rules over a source's ledger: the hash, the credits and the
// rate. The ledgers are built by hand, because what is tested is the mirror
// and not any one source.

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// asOf is a fixed instant the cases measure against.
var asOf = time.Date(2026, time.August, 1, 0, 0, 0, 0, time.UTC)

// creditedLedger is two invoices and a partial credit note against the first.
func creditedLedger() SourceLedger {
	return SourceLedger{Invoices: []SourceInvoice{
		{ExternalID: "INV-1", GrossMinor: 100000},
		{ExternalID: "INV-2", GrossMinor: 50000},
		{ExternalID: "CN-1", GrossMinor: 25000, CreditsExternalID: "INV-1"},
	}}
}

// The hash rule. An invoice's status depends on today, so a hash over it would
// change at midnight. Every morning's pass would then rewrite an untouched
// ledger, bump every row's version and report change where none happened.
func TestTheHashIgnoresEverythingDerivedFromToday(t *testing.T) {
	due := time.Date(2026, time.June, 1, 0, 0, 0, 0, time.UTC)
	invoice := SourceInvoice{
		ExternalID: "INV-1", Number: "INV-1",
		IssuedOn: due.AddDate(0, 0, -30), DueOn: &due,
		Currency: "EUR", NetMinor: 100000, GrossMinor: 119000,
	}
	before := invoiceHash(invoice)

	// The same invoice, read on a day when it has become overdue. Its DERIVED
	// status differs; the source said nothing new.
	early := deriveStatus(invoice, invoice.GrossMinor, due.AddDate(0, 0, -1))
	late := deriveStatus(invoice, invoice.GrossMinor, due.AddDate(0, 0, 1))
	if early == late {
		t.Fatal("the status did not change across the due date; this test proves nothing")
	}
	if after := invoiceHash(invoice); after != before {
		t.Fatal("the hash moved with the derived status: every pass would rewrite the ledger")
	}
}

// FIN-FORM-1 reads `credited_minor` off the invoice being reduced: its term is
// `net - credited`. A credit note that carried its own amount there would
// subtract nothing, and the credit would never reach the figure.
func TestACreditLandsOnTheInvoiceItReducesRatherThanOnTheNote(t *testing.T) {
	ledger := creditedLedger()
	credited, orphans := applyCredits(ledger)
	if len(orphans) != 0 {
		t.Fatalf("generated ledger has %d orphan credit notes", len(orphans))
	}
	if len(credited) == 0 {
		t.Fatal("no credit landed on any invoice")
	}
	for _, inv := range ledger.Invoices {
		if inv.CreditsExternalID == "" {
			continue
		}
		if credited[inv.ExternalID] != 0 {
			t.Fatalf("the credit landed on the note %s rather than on its target",
				inv.ExternalID)
		}
		if credited[inv.CreditsExternalID] != inv.GrossMinor {
			t.Fatalf("target %s carries %d credited, want the note's %d",
				inv.CreditsExternalID, credited[inv.CreditsExternalID], inv.GrossMinor)
		}
	}
}

// The source may list a note before the invoice it reduces. Credits resolve
// over the whole ledger, so that order changes nothing. A per-row pass would
// leave the note pointing at nothing, and the unchanged hash would stop a
// later pass from repairing it.
func TestCreditsResolveWhateverOrderTheSourceListsThemIn(t *testing.T) {
	target := SourceInvoice{ExternalID: "INV-1", GrossMinor: 100000}
	note := SourceInvoice{ExternalID: "CN-1", GrossMinor: 25000, CreditsExternalID: "INV-1"}

	noteFirst, _ := applyCredits(SourceLedger{Invoices: []SourceInvoice{note, target}})
	targetFirst, _ := applyCredits(SourceLedger{Invoices: []SourceInvoice{target, note}})
	if noteFirst["INV-1"] != 25000 || targetFirst["INV-1"] != 25000 {
		t.Fatalf("order changed the result: %d vs %d", noteFirst["INV-1"], targetFirst["INV-1"])
	}
}

// A note whose target the source did not send is reported rather than dropped:
// a credit that vanishes overstates what the customer owes.
func TestAnOrphanCreditIsReportedRatherThanDropped(t *testing.T) {
	note := SourceInvoice{ExternalID: "CN-9", GrossMinor: 5000, CreditsExternalID: "INV-GONE"}
	credited, orphans := applyCredits(SourceLedger{Invoices: []SourceInvoice{note}})
	if len(orphans) != 1 || orphans[0] != "CN-9" {
		t.Fatalf("orphans = %v, want [CN-9]", orphans)
	}
	if len(credited) != 0 {
		t.Fatalf("credited = %v, want nothing applied", credited)
	}
}

// A credit note is money going the other way. Left with its gross as an open
// balance it would inflate receivables by the amount it was meant to reduce.
func TestACreditNoteOwesNothing(t *testing.T) {
	note := SourceInvoice{
		ExternalID: "CN-1", GrossMinor: 25000, CreditsExternalID: "INV-1",
	}
	values := deriveValues(note, asOf, map[string]ids.UUID{}, 0, "EUR")
	if values.openMinor != 0 {
		t.Fatalf("the credit note owes %d, want 0", values.openMinor)
	}
	if values.status != statusCredited {
		t.Fatalf("status = %q, want %q", values.status, statusCredited)
	}
}

// The rate and the date it froze on are one fact (FIN-PARAM-7), and the schema
// enforces it. Only the identity is knowable without a rate sheet: an invoice
// already issued in the workspace's reporting currency is already in base.
func TestOnlyTheIdentityRateIsKnownWithoutARateSheet(t *testing.T) {
	issued := asOf
	for _, tc := range []struct {
		name     string
		currency string
		base     string
		want     bool
	}{
		{"same currency converts at one", "EUR", "EUR", true},
		{"case is not a different currency", "eur", "EUR", true},
		{"a foreign currency has no knowable rate", "USD", "EUR", false},
		{"an unknown base has no knowable rate", "EUR", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rate, when := fxRateToBase(
				SourceInvoice{Currency: tc.currency, IssuedOn: issued}, tc.base,
			)
			if tc.want {
				if rate == nil || *rate != 1 {
					t.Fatalf("rate = %v, want 1", rate)
				}
				// Both or neither: a rate without its date fails the schema's
				// own CHECK. A test that asserted only the rate would pass while
				// every insert aborted.
				if when == nil || !when.Equal(issued) {
					t.Fatalf("rate date = %v, want the issue date %v", when, issued)
				}
				return
			}
			if rate != nil || when != nil {
				t.Fatalf("invented a rate for %q against base %q: %v @ %v",
					tc.currency, tc.base, rate, when)
			}
		})
	}
}
