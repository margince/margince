// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mailer

import "errors"

// NotTakenError is a send the relay never took: it could not be reached, or
// it refused the exchange before the message's final line. SMTP keeps no
// half-taken message, so sending again cannot make a second copy. A failure
// on that final line is not one: the relay may hold the message already.
//
// Error is the cause's own text, so a caller that stores it stores what it
// stored before.
type NotTakenError struct{ Err error }

func (e *NotTakenError) Error() string { return e.Err.Error() }

func (e *NotTakenError) Unwrap() error { return e.Err }

// NotTaken reports whether err is a send the relay never took, so a caller
// may try it again without risking a second message.
func NotTaken(err error) bool {
	var refused *NotTakenError
	return errors.As(err, &refused)
}
