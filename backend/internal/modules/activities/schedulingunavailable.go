// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"errors"
	"log/slog"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// calendarReadFailure turns a provider's failure to report a calendar's
// occupancy into one answer for every reader of busy time. The booking page,
// the proposal page, the assistant tool and the reschedule check all read it
// through calendarBusy, so their verdicts cannot differ.
//
// A refused credential, a permission refusal and a caller who left are not the
// calendar's fault, so they keep their own answers. Any other failure leaves
// the host's diary unknown, and availability fails closed on it.
func calendarReadFailure(ctx context.Context, host ids.UserID, provider, calendar string, err error) error {
	if ctx.Err() != nil || errors.Is(err, connector.ErrAuthRejected) || errors.Is(err, apperrors.ErrPermissionDenied) {
		return err
	}
	unavailable, ok := errors.AsType[*connector.CalendarUnavailableError](err)
	if !ok {
		unavailable = connector.NewCalendarUnavailable(calendar, "", err)
	}
	// Warn, not error: the host repairs their own diary, and nothing failed in
	// this process. The line lets an operator tell a deleted calendar from a
	// revoked grant from a provider outage.
	slog.WarnContext(ctx, "availability refused: selected calendar could not be checked",
		"host_id", host.UUID, "provider", provider, "calendar", calendar,
		"reason", unavailable.Reason, "err", err)
	return unavailable
}
