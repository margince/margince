// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// spacedReceipts is n acts a minute apart, newest first.
func spacedReceipts(n int) listedReceipts {
	out := make(listedReceipts, 0, n)
	for i := range n {
		out = append(out, Receipt{ID: ids.NewV7(), OccurredAt: rankInstant.Add(-time.Duration(i) * time.Minute)})
	}
	return out
}

func TestASecondPageCarriesEveryReceiptTheFirstLeftOut(t *testing.T) {
	t.Parallel()
	acts := spacedReceipts(handledCap + 10)
	svc := &Service{now: func() time.Time { return rankInstant }, receipts: acts}

	first, err := svc.HandledForYou(leadReader(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !first.Truncated || first.NextCursor == nil {
		t.Fatalf("a truncated page hands back no cursor: truncated=%t", first.Truncated)
	}
	second, err := svc.HandledForYou(leadReader(), first.NextCursor)
	if err != nil {
		t.Fatal(err)
	}
	if second.Truncated || second.NextCursor != nil || len(second.Receipts) != 10 {
		t.Fatalf("second page: %d receipts, truncated=%t; want the 10 left and the end", len(second.Receipts), second.Truncated)
	}
	for i, receipt := range append(first.Receipts, second.Receipts...) {
		if ids.UUID(receipt.Id) != acts[i].ID {
			t.Fatalf("receipt %d is %s, want %s: the pages skip or repeat", i, receipt.Id, acts[i].ID)
		}
	}
	if !second.Since.Equal(*first.Since) {
		t.Errorf("the second page reads from %v, the first from %v", second.Since, first.Since)
	}
}

func TestAReceiptLandingBetweenPagesDoesNotShiftTheNext(t *testing.T) {
	t.Parallel()
	acts := spacedReceipts(handledCap + 10)
	svc := &Service{now: func() time.Time { return rankInstant }, receipts: acts}
	first, err := svc.HandledForYou(leadReader(), nil)
	if err != nil {
		t.Fatal(err)
	}

	landed := Receipt{ID: ids.NewV7(), OccurredAt: rankInstant.Add(time.Minute)}
	svc.receipts = append(listedReceipts{landed}, acts...)
	second, err := svc.HandledForYou(leadReader(), first.NextCursor)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Receipts) != 10 || ids.UUID(second.Receipts[0].Id) != acts[handledCap].ID {
		t.Fatalf("after a new receipt the second page starts at %v with %d receipts, want %s with 10",
			second.Receipts[0].Id, len(second.Receipts), acts[handledCap].ID)
	}
}

func TestAReceiptLandingBetweenPagesKeepsTheRestReachable(t *testing.T) {
	t.Parallel()
	acts := spacedReceipts(3 * handledCap)
	svc := &Service{now: func() time.Time { return rankInstant }, receipts: acts}
	first, err := svc.HandledForYou(leadReader(), nil)
	if err != nil {
		t.Fatal(err)
	}

	landed := Receipt{ID: ids.NewV7(), OccurredAt: rankInstant.Add(time.Minute)}
	svc.receipts = append(listedReceipts{landed}, acts...)
	second, err := svc.HandledForYou(leadReader(), first.NextCursor)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Receipts) != handledCap || !second.Truncated || second.NextCursor == nil {
		t.Fatalf("second page holds %d receipts, truncated %v, next cursor %v; want %d, true and a cursor",
			len(second.Receipts), second.Truncated, second.NextCursor, handledCap)
	}
}

func TestACursorThisServerDidNotMintIsRefused(t *testing.T) {
	t.Parallel()
	svc := &Service{now: func() time.Time { return rankInstant }, receipts: spacedReceipts(3)}
	forged, err := storekit.EncodeOpaque(handledCursor{Offset: 0, Since: rankInstant, Newest: rankInstant})
	if err != nil {
		t.Fatal(err)
	}
	beyond, err := storekit.EncodeOpaque(handledCursor{Offset: maxHandledOffset + 1, Since: rankInstant, Newest: rankInstant})
	if err != nil {
		t.Fatal(err)
	}
	for name, token := range map[string]string{"garbage": "not-a-cursor", "no offset": forged, "too deep": beyond} {
		if _, err := svc.HandledForYou(leadReader(), &token); err == nil {
			t.Errorf("%s: the cursor was read as a page", name)
		} else if _, malformed := errors.AsType[*storekit.MalformedCursorError](err); !malformed && name == "garbage" {
			t.Errorf("%s: answered %v, want the malformed-cursor refusal", name, err)
		}
	}
}

func TestASourceTheReaderMayNotReadIsLeftOff(t *testing.T) {
	t.Parallel()
	readable, hidden := ids.NewV7(), ids.NewV7()
	svc := &Service{
		now: func() time.Time { return rankInstant },
		receipts: listedReceipts{
			{ID: ids.NewV7(), SourceActivityID: readable},
			{ID: ids.NewV7(), SourceActivityID: hidden},
			{ID: ids.NewV7()},
		},
		names: &stubNames{labels: map[ids.UUID]string{readable: "Renewal call"}},
	}
	out, err := svc.HandledForYou(leadReader(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if source := out.Receipts[0].SourceRecord; source == nil || source.Label == nil || *source.Label != "Renewal call" {
		t.Errorf("a readable source reads %+v", source)
	}
	if source := out.Receipts[1].SourceRecord; source != nil {
		t.Errorf("a source the reader may not read is named: %+v", source)
	}
	if source := out.Receipts[2].SourceRecord; source != nil {
		t.Errorf("an act citing no message names one: %+v", source)
	}
}
