// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// calendarReadFailure gives a provider's failure to report a calendar's
// occupancy one answer for every reader of busy time. The booking page, the
// assistant tool, the reschedule check and the writes that re-check a slot all
// read it through calendarBusy, so their verdicts agree.
//
// Only a failure from the provider is the calendar's fault: the connector's own
// classification, or a provider that could not be reached. Anything else
// passes through, so it keeps its status and its error-level log line.
func calendarReadFailure(ctx context.Context, host ids.UserID, provider, calendar string, err error) error {
	if ctx.Err() != nil {
		return err
	}
	unavailable, ok := errors.AsType[*connector.CalendarUnavailableError](err)
	if !ok {
		if !errors.Is(err, connector.ErrUnreachable) || errors.Is(err, connector.ErrAuthRejected) || errors.Is(err, apperrors.ErrPermissionDenied) {
			return err
		}
		unavailable = connector.NewCalendarUnavailable(calendar, "unreachable", err)
	}
	// Warn, not error: the host repairs their own diary, and nothing failed in
	// this process. The line lets an operator tell a deleted calendar from a
	// revoked grant from a provider outage.
	slog.WarnContext(ctx, "availability refused: selected calendar could not be checked",
		"host_id", host.UUID, "provider", provider, "calendar", calendar,
		"reason", unavailable.Reason, "err", err)
	return unavailable
}

// calendarNamedToItsHost adds the unreadable calendar's id to the refusal when
// the signed-in host asks about their own calendars, so their preview can name
// it. The id is the host's account. An agent acting for the host, any other
// caller and every public door keep the fixed sentence alone.
func calendarNamedToItsHost(err error, actor principal.Principal, host ids.UserID) error {
	unavailable, ok := errors.AsType[*connector.CalendarUnavailableError](err)
	if !ok || !isTheHostInPerson(actor, host) || unavailable.Calendar == "" {
		return err
	}
	_, message := unavailable.MessageFault()
	return &httperr.DetailedError{
		Status: http.StatusBadGateway, Code: connector.CalendarUnavailableCode, Detail: message,
		Details: map[string]any{"calendar": unavailable.Calendar},
	}
}

// isTheHostInPerson says the caller is the host, signed in themselves. An agent
// passport carries its granting human's id, so the type decides too.
func isTheHostInPerson(actor principal.Principal, host ids.UserID) bool {
	return actor.Type == principal.PrincipalHuman && !actor.UserID.IsZero() && actor.UserID == host.UUID
}
