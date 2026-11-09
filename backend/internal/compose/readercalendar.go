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
	"log/slog"
	"slices"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
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
// calendar, and whether it owes anything. A list that holds meetings, or a time
// window over every kind, reads as the reader's diary. A list held to one record, project
// or assignee does not. A reader refused their own connections is owed nothing.
func activityListCalendar(registry *capture.Registry) func(ctx context.Context, scope datasource.ActivityListScope) (calendar crmcontracts.ReaderCalendar, owed bool, err error) {
	return func(ctx context.Context, scope datasource.ActivityListScope) (crmcontracts.ReaderCalendar, bool, error) {
		readsAsDiary := slices.Contains(scope.Kinds, string(crmcontracts.ActivityKindMeeting)) || (len(scope.Kinds) == 0 && scope.Windowed)
		if !readsAsDiary || scope.Narrowed {
			return "", false, nil
		}
		calendar, err := readerCalendar(ctx, registry)
		switch {
		case errors.Is(err, apperrors.ErrPermissionDenied):
			return "", false, nil
		case err != nil:
			// The rows are already read; a failed side read leaves the field
			// absent, which reads as unknown, rather than failing the list.
			slog.ErrorContext(ctx, "activity list: reading the reader's calendar connections", "err", err)
			return "", false, nil
		}
		return calendar, true, nil
	}
}
