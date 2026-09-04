// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// What a reader is shown about work already done.

import (
	"context"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The reader these acts were taken for. leadReader is reused rather than
// respelled: a receipt needs a human behind the call and nothing more, and a
// second spelling of one principal is a second thing to keep right.

// TestAReceiptNeverBecomesAnObligation is the property the whole surface rests
// on.
//
// A receipt that came back as a row would ask the reader to redo what was
// already done — and it would do so on the one page they trust to be a list of
// things outstanding. The lane is kept out of the worklist's own vocabulary,
// and this holds that: `done_for_you` is not a source the queue can emit.
func TestAReceiptNeverBecomesAnObligation(t *testing.T) {
	t.Parallel()
	if crmcontracts.WorklistItemSource("done_for_you").Valid() {
		t.Error("the queue can emit done_for_you, so a completed act can come back as " +
			"work the reader is asked to do again")
	}
}

// TestABoundedReadSaysSoRatherThanLookingComplete.
//
// A page that stopped at its own bound and did not say so reads as the whole of
// what was done. The reader closes it believing they have seen everything, which
// is the one thing a receipt surface must not cause.
func TestABoundedReadSaysSoRatherThanLookingComplete(t *testing.T) {
	t.Parallel()
	svc := &Service{
		now:      func() time.Time { return rankInstant },
		receipts: fixedReceipts(handledCap + 1),
	}

	out, err := svc.HandledForYou(leadReader())
	if err != nil {
		t.Fatalf("reading the receipts: %v", err)
	}

	if !out.Truncated {
		t.Error("a read that stopped at its bound reports itself complete")
	}
	if len(out.Receipts) != handledCap {
		t.Errorf("drew %d receipts, want the page's own bound of %d",
			len(out.Receipts), handledCap)
	}
}

// TestAFullPageIsNotABoundedOne separates the two states the cap can produce.
//
// Reading exactly the cap and reading past it look identical from a count
// alone, which is why the read asks for one more than it draws. Without that a
// page holding exactly fifty would claim there was more when there was not.
func TestAFullPageIsNotABoundedOne(t *testing.T) {
	t.Parallel()
	svc := &Service{
		now:      func() time.Time { return rankInstant },
		receipts: fixedReceipts(handledCap),
	}

	out, err := svc.HandledForYou(leadReader())
	if err != nil {
		t.Fatalf("reading the receipts: %v", err)
	}

	if out.Truncated {
		t.Error("a page holding exactly the bound claims there is more behind it")
	}
	if len(out.Receipts) != handledCap {
		t.Errorf("drew %d receipts, want all %d", len(out.Receipts), handledCap)
	}
}

// TestAnUnwiredReaderShowsNothingRatherThanRefusing.
//
// An installation that wires no receipt reader did nothing on anybody's behalf
// to report. That is honestly an empty page, not a surface the reader may not
// see — and a refusal would tell them something exists which does not.
func TestAnUnwiredReaderShowsNothingRatherThanRefusing(t *testing.T) {
	t.Parallel()
	svc := &Service{now: func() time.Time { return rankInstant }}

	out, err := svc.HandledForYou(leadReader())
	if err != nil {
		t.Fatalf("an unwired reader refused the page: %v", err)
	}
	if len(out.Receipts) != 0 || out.Truncated {
		t.Errorf("an unwired reader reported %d receipts (truncated=%v), want an empty page",
			len(out.Receipts), out.Truncated)
	}
}

// fixedReceipts is n completed acts, newest first.
type fixedReceipts int

func (n fixedReceipts) Recent(_ context.Context, _ time.Time, limit int) ([]Receipt, error) {
	out := []Receipt{}
	for i := 0; i < int(n) && len(out) < limit; i++ {
		out = append(out, Receipt{
			ID:         ids.NewV7(),
			Kind:       "email_sent",
			Summary:    "Sent the confirmation",
			OccurredAt: rankInstant,
		})
	}
	return out, nil
}
