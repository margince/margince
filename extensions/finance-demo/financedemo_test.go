// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package financedemo

import (
	"context"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/margince/margince/backend/pkg/extension/accounting"
)

// epoch is a fixed anchor, so the assertions measure the generator rather than
// how long ago a date was.
var epoch = time.Date(2026, time.August, 1, 0, 0, 0, 0, time.UTC)

func ledgerAt(t *testing.T, connectionID string, anchor time.Time, customer string) accounting.Ledger {
	t.Helper()
	ledger, err := invoicesFor(context.Background(), nil,
		accounting.Connection{ID: connectionID, ConnectedAt: anchor}, customer)
	if err != nil {
		t.Fatal(err)
	}
	return ledger
}

// eachLedger runs check over many seeds: a property of "every generated
// customer" is a property of the whole seed space, not of a few names.
func eachLedger(t *testing.T, check func(t *testing.T, ledger accounting.Ledger)) {
	t.Helper()
	for c := range 8 {
		for n := 1; n <= 8; n++ {
			connection, customer := fmt.Sprintf("conn-%d", c), fmt.Sprintf("DEMO-%04d", n)
			t.Run(connection+"/"+customer, func(t *testing.T) {
				check(t, ledgerAt(t, connection, epoch, customer))
			})
		}
	}
}

func TestTheDeclarationIsAValidFinanceSource(t *testing.T) {
	ext := New()
	if err := ext.Name.Validate(); err != nil {
		t.Fatal(err)
	}
	if len(ext.FinanceSources) != 1 {
		t.Fatalf("declares %d finance sources, want 1", len(ext.FinanceSources))
	}
	if err := ext.FinanceSources[0].Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestTheDeclaredRecordKindIsValidForThisUnit(t *testing.T) {
	ext := New()
	if len(ext.RecordKinds) != 1 {
		t.Fatalf("declares %d record kinds, want 1", len(ext.RecordKinds))
	}
	if err := ext.RecordKinds[0].Validate(ext.Name); err != nil {
		t.Fatal(err)
	}
}

func TestTheDeclarationIsAValidOAuthApp(t *testing.T) {
	apps := New().OAuthApps
	if len(apps) != 1 {
		t.Fatalf("declares %d OAuth apps, want 1", len(apps))
	}
	if err := apps[0].Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestTheDirectoryListsDistinctCustomers(t *testing.T) {
	listed, err := customers(context.Background(), nil, accounting.Connection{})
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, c := range listed {
		if seen[c.ExternalID] {
			t.Fatalf("customer %s listed twice", c.ExternalID)
		}
		seen[c.ExternalID] = true
	}
	if len(seen) != directorySize {
		t.Fatalf("listed %d customers, want %d", len(seen), directorySize)
	}
}

// A sync rewrites a row only when the source changed. So one anchor must give
// the same ledger every time, and so must an anchor hours later that day.
func TestTheLedgerIsTheSameOnEveryGeneration(t *testing.T) {
	first := ledgerAt(t, "conn-1", epoch, "DEMO-0001")
	if !reflect.DeepEqual(first, ledgerAt(t, "conn-1", epoch, "DEMO-0001")) {
		t.Error("two generations at one anchor differ — every sync would rewrite every row")
	}
	if !reflect.DeepEqual(first, ledgerAt(t, "conn-1", epoch.Add(14*time.Hour), "DEMO-0001")) {
		t.Error("two anchors on one UTC day produced different ledgers")
	}
}

func TestTwoCustomersGetDifferentLedgers(t *testing.T) {
	a := ledgerAt(t, "conn-1", epoch, "DEMO-0001")
	b := ledgerAt(t, "conn-1", epoch, "DEMO-0002")
	if reflect.DeepEqual(a.Invoices, b.Invoices) {
		t.Fatal("two customers generated the same invoices")
	}
}

// A later connection gets the same shape, shifted by the difference, so the
// card's rolling windows find the same invoices whenever it was made.
func TestTheLedgerShiftsWithItsAnchorAndKeepsItsShape(t *testing.T) {
	shift := 400 * 24 * time.Hour
	early := ledgerAt(t, "conn-1", epoch, "DEMO-0001")
	late := ledgerAt(t, "conn-1", epoch.Add(shift), "DEMO-0001")
	if len(early.Invoices) != len(late.Invoices) {
		t.Fatalf("invoice counts differ across anchors: %d vs %d", len(early.Invoices), len(late.Invoices))
	}
	for i, want := range early.Invoices {
		got := late.Invoices[i]
		if got.ExternalID != want.ExternalID || got.NetMinor != want.NetMinor {
			t.Fatalf("invoice %d differs beyond its dates: %+v vs %+v", i, got, want)
		}
		if diff := got.IssuedOn.Sub(want.IssuedOn); diff != shift {
			t.Errorf("invoice %s moved %v, want %v", got.ExternalID, diff, shift)
		}
	}
}

// Nothing may have happened after the anchor, or the ledger tracks the clock.
// A due date may fall after it: that is a promise, not an event.
func TestNoRowIsDatedAfterTheAnchor(t *testing.T) {
	eachLedger(t, func(t *testing.T, ledger accounting.Ledger) {
		for _, inv := range ledger.Invoices {
			if inv.IssuedOn.After(epoch) {
				t.Fatalf("row %s is issued %s, after the anchor", inv.ExternalID, inv.IssuedOn.Format(time.DateOnly))
			}
			if inv.FullyPaidAt != nil && inv.FullyPaidAt.After(epoch) {
				t.Fatalf("row %s is paid %s, after the anchor", inv.ExternalID, inv.FullyPaidAt.Format(time.DateOnly))
			}
		}
		for _, p := range ledger.Payments {
			if p.PaidAt.After(epoch) {
				t.Fatalf("payment %s is dated %s, after the anchor", p.ExternalID, p.PaidAt.Format(time.DateOnly))
			}
		}
	})
}

// settledInWindow is the demo's promise to the card. The median days-to-pay
// needs five settled, undisputed invoices due in the last 180 days. Every
// generated customer carries at least eight, so one more dispute cannot tip it
// into the refusal.
const settledInWindow = 8

func TestEveryCustomerCarriesATimelinessSample(t *testing.T) {
	windowStart := epoch.AddDate(0, 0, -180)
	eachLedger(t, func(t *testing.T, ledger accounting.Ledger) {
		settled := 0
		for _, inv := range ledger.Invoices {
			if inv.FullyPaidAt != nil && !inv.Disputed && inv.DueOn != nil &&
				!inv.DueOn.Before(windowStart) && !inv.DueOn.After(epoch) {
				settled++
			}
		}
		if settled < settledInWindow {
			t.Fatalf("%d settled invoices due in the window, want at least %d", settled, settledInWindow)
		}
	})
}

func TestEveryCustomerOwesAnOpenBalance(t *testing.T) {
	eachLedger(t, func(t *testing.T, ledger accounting.Ledger) {
		open := int64(0)
		for _, inv := range ledger.Invoices {
			if inv.CreditsExternalID == "" {
				open += inv.GrossMinor - inv.PaidMinor
			}
		}
		if open <= 0 {
			t.Fatal("the ledger owes nothing; the open-balance reading has nothing to show")
		}
	})
}

func TestTheCreditNoteReducesAnInvoiceInTheLedger(t *testing.T) {
	eachLedger(t, func(t *testing.T, ledger accounting.Ledger) {
		byID := map[string]bool{}
		for _, inv := range ledger.Invoices {
			byID[inv.ExternalID] = true
		}
		notes := 0
		for _, inv := range ledger.Invoices {
			if inv.CreditsExternalID == "" {
				continue
			}
			notes++
			if !byID[inv.CreditsExternalID] {
				t.Fatalf("credit note %s reduces %s, which is not in the ledger", inv.ExternalID, inv.CreditsExternalID)
			}
		}
		if notes == 0 {
			t.Fatal("no credit note; the negative term is never exercised")
		}
	})
}
