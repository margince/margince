// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type countedReceipts struct{ reads *int }

func (c countedReceipts) Recent(context.Context, time.Time, int) ([]Receipt, error) {
	*c.reads++
	return nil, nil
}

func TestTheRankedQueueLeavesTheReceiptLaneUnread(t *testing.T) {
	reads := 0
	svc := NewService(stubApprovals{}, stubDuplicates{}, &stubTasks{}, countedReceipts{&reads},
		stubBriefing{}, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, fixedClock)

	if _, err := svc.Worklist(pageReader(), "", "", ids.UUID{}, 25, ""); err != nil {
		t.Fatalf("worklist: %v", err)
	}
	if reads != 0 {
		t.Errorf("the worklist read receipts %d times, want none: no receipt is a queue row", reads)
	}
	if _, err := svc.Assemble(pageReader()); err != nil {
		t.Fatalf("assembling: %v", err)
	}
	if reads != 1 {
		t.Errorf("the lane feed read receipts %d times, want once", reads)
	}
}
