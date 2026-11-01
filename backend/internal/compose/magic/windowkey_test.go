// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package magic

import (
	"context"
	"testing"
	"time"
)

type briefAt struct {
	cutoff time.Time
	ran    bool
}

func (b briefAt) CutoffFor(context.Context) (time.Time, bool, error) { return b.cutoff, b.ran, nil }

// A cached receipt is keyed by its window, so a brief that runs moves the key.
func TestTheReceiptWindowKeyFollowsTheReadersBrief(t *testing.T) {
	now := time.Date(2026, 11, 2, 9, 30, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	cutoff := time.Date(2026, 11, 2, 6, 40, 0, 0, time.UTC)

	before, err := NewService(nil, briefAt{}, clock).windowKey(context.Background())
	if err != nil || before != "day" {
		t.Fatalf("before any brief: key %q (err %v), want the rolling day", before, err)
	}
	after, err := NewService(nil, briefAt{cutoff: cutoff, ran: true}, clock).windowKey(context.Background())
	if err != nil || after != cutoff.Format(time.RFC3339Nano) {
		t.Fatalf("after the brief: key %q (err %v), want its cutoff", after, err)
	}
}
