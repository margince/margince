// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package connector

import (
	"context"
	"errors"
	"fmt"

	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// CalendarUnavailableCode is the machine code for a host calendar that could
// not be checked. It tells a visitor and a host that no times are offered
// because of the calendar, not because of a fault nobody can name.
const CalendarUnavailableCode = "calendar_unavailable"

// CalendarUnavailableError says a selected calendar's occupancy could not be
// established. The provider reported an error, left it out, or answered in a
// shape that cannot be trusted. A partial picture would double-book the host,
// so availability refuses instead.
//
// Calendar and Reason are for the operator's log only. The client gets the
// fixed sentence from MessageFault, because the calendar id is the host's
// account and the reason is text a remote party chose.
type CalendarUnavailableError struct {
	Calendar string
	// Reason is the provider's machine code, or a short label when it named
	// none. It always passes MachineReason, so a log line can carry it.
	Reason string
	Cause  error
}

// NewCalendarUnavailable bounds reason through MachineReason, because the
// provider controls the body it is parsed out of.
func NewCalendarUnavailable(calendar, reason string, cause error) *CalendarUnavailableError {
	return &CalendarUnavailableError{Calendar: calendar, Reason: MachineReason(reason), Cause: cause}
}

// CalendarReadFailed classifies a failure out of a provider's busy-time answer.
// A shape the connector cannot trust becomes the calendar's fault. Examples are
// an unreadable body, an interval that ends before it starts and a page chain
// that never ends. A refused credential, an unreachable provider and a
// cancelled request keep their own classes, which the caller already acts on.
func CalendarReadFailed(calendar string, err error) error {
	if err == nil || errors.Is(err, ErrAuthRejected) || errors.Is(err, ErrUnreachable) ||
		errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	if _, already := errors.AsType[*CalendarUnavailableError](err); already {
		return err
	}
	return NewCalendarUnavailable(calendar, "", err)
}

func (e *CalendarUnavailableError) Error() string {
	text := fmt.Sprintf("calendar: selected calendar %q could not be checked", e.Calendar)
	if e.Reason != "" {
		text += " (" + e.Reason + ")"
	}
	if e.Cause != nil {
		text += ": " + e.Cause.Error()
	}
	return text
}

func (e *CalendarUnavailableError) Unwrap() error { return e.Cause }

// Is makes this the shared "an outside service gave no usable answer"
// condition. The status and a job's failure class then follow from the
// sentinel, and this port names neither.
func (e *CalendarUnavailableError) Is(target error) bool {
	return target == apperrors.ErrProviderUnusable
}

// MessageFault gives both readers a sentence they can use. The visitor learns
// why no times appear, and the host learns where the fix is.
func (e *CalendarUnavailableError) MessageFault() (code, message string) {
	return CalendarUnavailableCode, "The host's selected calendar could not be read, so nothing was offered or changed. " +
		"The host can check the calendar chosen in meeting settings."
}
