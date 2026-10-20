// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package connector

import (
	"fmt"

	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// CalendarUnavailableCode is the machine code for a host calendar that could
// not be checked. It tells a visitor and a host that no times are offered
// because of the calendar, not because of a fault nobody can name.
const CalendarUnavailableCode = "calendar_unavailable"

// CalendarUnavailableError says a selected calendar's occupancy could not be
// established. The provider reported an error for it, left it out, or answered
// in a shape that cannot be trusted. Slots on a partial picture would
// double-book the host, so every availability read refuses instead.
//
// Calendar and Reason are for the operator's log only. The client gets the
// fixed sentence from MessageFault: the calendar id is the host's account, and
// the reason is text a remote party chose.
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
	return CalendarUnavailableCode, "No times can be offered because the host's selected calendar could not be read. " +
		"The host can check the calendar chosen in scheduling settings, and anyone else can try again later."
}
