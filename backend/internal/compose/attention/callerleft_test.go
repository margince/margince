// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// waitingThenLeave answers the who-is-waiting lane and then hangs up, the way a
// reader who navigates away mid-read cancels the request under every later lane.
type waitingThenLeave struct {
	waitingOwnedBy
	leave context.CancelFunc
}

func (w waitingThenLeave) Unanswered(ctx context.Context, at time.Time, by WaitingFor) ([]WaitingCustomer, bool, error) {
	w.leave()
	return w.waitingOwnedBy.Unanswered(ctx, at, by)
}

func TestACallerWhoLeavesGetsNoPageAndFreezesNoWalk(t *testing.T) {
	t.Parallel()
	ctx, leave := context.WithCancel(managerReading())
	defer leave()
	walks := &walkStore{}
	svc := NewService(
		stubApprovals{}, stubDuplicates{}, &stubTasks{}, stubReceipts{},
		stubBriefing{}, nil, nil, nil, nil,
		nil, nil, nil, nil, nil, nil, nil, nil, fixedClock).WithWalks(walks)
	svc.waiting = waitingThenLeave{
		waitingOwnedBy: waitingOwnedBy{{
			ActivityID: ids.NewV7(), Subject: "a customer", OwnerID: theManager,
			Since: readInstant.Add(-48 * time.Hour),
		}},
		leave: leave,
	}

	_, err := svc.Worklist(ctx, "", "", ids.UUID{}, 2, "")

	if !errors.Is(err, context.Canceled) {
		t.Errorf("Worklist returned %v after the caller left, want context.Canceled", err)
	}
	if len(walks.kept) != 0 {
		t.Errorf("froze %d walks for a caller who left; each is a snapshot write nobody reads", len(walks.kept))
	}
}
