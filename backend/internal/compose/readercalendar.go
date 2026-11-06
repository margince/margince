// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Whether the reader's own calendar feeds the meetings this product holds. The
// Worklist and both doors of the activity list answer from here, so a page and
// a tool cannot disagree about one connection.

import (
	"context"
	"errors"
	"fmt"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

func readerCalendar(ctx context.Context, registry *capture.Registry) (crmcontracts.ReaderCalendar, error) {
	reach, err := registry.ReachOn(ctx, calendarProviders)
	if err != nil {
		return "", err
	}
	switch reach {
	case capture.ReachLive:
		return crmcontracts.ReaderCalendarConnected, nil
	case capture.ReachBroken:
		return crmcontracts.ReaderCalendarUnreadable, nil
	case capture.ReachNone:
		return crmcontracts.ReaderCalendarNotConnected, nil
	default:
		return "", fmt.Errorf("compose: calendar reach %q has no reader-calendar answer", reach)
	}
}

// activityListCalendar is what an activity list owes about the reader's
// calendar, and whether it owes anything. A list narrowed to meetings, or to a time
// window over every kind, reads as the reader's diary; any other list does not.
// A reader refused their own connections is owed nothing, as on the Worklist.
func activityListCalendar(registry *capture.Registry) func(ctx context.Context, kind string, windowed bool) (calendar crmcontracts.ReaderCalendar, owed bool, err error) {
	return func(ctx context.Context, kind string, windowed bool) (crmcontracts.ReaderCalendar, bool, error) {
		readsAsDiary := kind == string(crmcontracts.ActivityKindMeeting) || (kind == "" && windowed)
		if !readsAsDiary {
			return "", false, nil
		}
		calendar, err := readerCalendar(ctx, registry)
		if errors.Is(err, apperrors.ErrPermissionDenied) {
			return "", false, nil
		}
		return calendar, err == nil, err
	}
}
