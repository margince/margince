// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// waitingAsked records how each read of the who-is-waiting lane was narrowed.
type waitingAsked struct {
	waitingOwnedBy
	asked *[]WaitingFor
}

func (w waitingAsked) Unanswered(_ context.Context, _ time.Time, narrow WaitingFor) ([]WaitingCustomer, bool, error) {
	*w.asked = append(*w.asked, narrow)
	return nil, false, nil
}

// Each scope narrows the waiting scan to the rows its own filter keeps. The
// team scope narrows to the roster and former teammates the team filter reads,
// so the scan bound is spent on the team's mail.
func TestEachScopeNarrowsTheWaitingScanToTheRowsItKeeps(t *testing.T) {
	t.Parallel()
	departed := ids.MustParse("01a05500-0000-7000-8000-0000000000a3")
	for _, tc := range []struct {
		scope string
		want  WaitingFor
	}{
		{scopeTeam, WaitingFor{Team: true, Seats: []ids.UUID{theManager, theRep, departed}}},
		{scopeAll, WaitingFor{}},
		{scopeMine, WaitingFor{Owner: &theManager}},
	} {
		t.Run(tc.scope, func(t *testing.T) {
			t.Parallel()
			var asked []WaitingFor
			svc := NewService(
				stubApprovals{}, stubDuplicates{}, &stubTasks{}, stubReceipts{},
				stubBriefing{}, nil, nil, nil, nil,
				nil, nil, nil, nil, nil, nil, nil, nil, fixedClock)
			svc.waiting = waitingAsked{asked: &asked}
			svc.teammates = roster{{UserID: theManager}, {UserID: theRep}}
			svc.former = &countedFormerRead{ids: []ids.UUID{departed}}
			if _, err := svc.Worklist(managerReading(), tc.scope, "", ids.Nil, 25, ""); err != nil {
				t.Fatalf("reading the %s worklist: %v", tc.scope, err)
			}
			if len(asked) == 0 {
				t.Fatal("the waiting lane was never read")
			}
			for _, got := range asked {
				if got.Team != tc.want.Team || !slices.Equal(got.Seats, tc.want.Seats) ||
					(got.Owner == nil) != (tc.want.Owner == nil) || (got.Owner != nil && *got.Owner != *tc.want.Owner) {
					t.Errorf("the %s scope read the waiting lane as %+v, want %+v", tc.scope, got, tc.want)
				}
			}
		})
	}
}
