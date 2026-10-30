// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestOneWorklistReadAsksForTheZoneOnce(t *testing.T) {
	asked := 0
	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatalf("loading the zone: %v", err)
	}
	svc := NewService(stubApprovals{}, stubDuplicates{}, &stubTasks{}, stubReceipts{},
		stubBriefing{}, nil, nil, nil, &stubMeetings{}, nil, nil, nil, nil, nil, nil, nil, nil, fixedClock,
		WithZone(func(context.Context) (*time.Location, error) {
			asked++
			return berlin, nil
		}))

	if _, err := svc.Worklist(pageReader(), "", "", ids.UUID{}, 25, ""); err != nil {
		t.Fatalf("worklist: %v", err)
	}
	if asked != 1 {
		t.Errorf("one read asked for the installation's zone %d times, want once", asked)
	}
}
