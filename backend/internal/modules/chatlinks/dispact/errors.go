// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dispact

import (
	"errors"
	"fmt"
)

var (
	// ErrTokenRejected: the chat refused the bot token, so the admin must
	// replace it.
	ErrTokenRejected = errors.New("dispact: the bot token was rejected")
	// ErrNotMember: the bot may not act in this channel.
	ErrNotMember = errors.New("dispact: the bot is not a member of the channel")
	// ErrChannelGone: the channel or message no longer exists.
	ErrChannelGone = errors.New("dispact: the channel is gone")
	// ErrUnreachable: the chat could not be reached or answered with a fault.
	ErrUnreachable = errors.New("dispact: the chat could not be reached")
	// ErrRequestRejected: the chat refused the request as malformed.
	ErrRequestRejected = errors.New("dispact: the chat refused the request")
)

// StatusError keeps the HTTP status of a refusal next to its typed meaning.
// A caller that needs the exact status, such as a 409 on create, reads it.
type StatusError struct {
	Route   string
	Status  int
	wrapped error
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("dispact: %s: status %d: %v", e.Route, e.Status, e.wrapped)
}

func (e *StatusError) Unwrap() error { return e.wrapped }

func statusOf(err error) (int, bool) {
	if status, ok := errors.AsType[*StatusError](err); ok {
		return status.Status, true
	}
	return 0, false
}
