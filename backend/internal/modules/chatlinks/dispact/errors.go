// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dispact

import (
	"errors"
	"fmt"
)

var (
	// ErrTokenRejected means the chat refused the bot token, so the admin must
	// replace it.
	ErrTokenRejected = errors.New("dispact: the bot token was rejected")
	// ErrNotMember means the bot may not act in this channel.
	ErrNotMember = errors.New("dispact: the bot is not a member of the channel")
	// ErrChannelGone means the channel or message no longer exists.
	ErrChannelGone = errors.New("dispact: the channel is gone")
	// ErrUnreachable means the chat could not be reached or answered with a fault.
	ErrUnreachable = errors.New("dispact: the chat could not be reached")
	// ErrRequestRejected means the chat refused the request as malformed, or as
	// one it will not do, such as a name already taken.
	ErrRequestRejected = errors.New("dispact: the chat refused the request")
)

// StatusError keeps the HTTP status of a refusal next to its typed meaning.
// A log then names the status a proxy in front of Dispact answered.
type StatusError struct {
	Route   string
	Status  int
	wrapped error
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("dispact: %s: status %d: %v", e.Route, e.Status, e.wrapped)
}

func (e *StatusError) Unwrap() error { return e.wrapped }

// CodeError keeps the error code of a refusal Dispact sent with HTTP 200
// next to its typed meaning. The code is Dispact's fixed vocabulary, never
// channel content, so it may reach a log.
type CodeError struct {
	Route   string
	Code    string
	wrapped error
}

func (e *CodeError) Error() string {
	return fmt.Sprintf("dispact: %s: refused with %q: %v", e.Route, e.Code, e.wrapped)
}

func (e *CodeError) Unwrap() error { return e.wrapped }
