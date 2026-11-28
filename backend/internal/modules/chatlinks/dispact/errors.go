// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dispact

import (
	"fmt"

	"github.com/margince/margince/backend/internal/modules/chatlinks/chatwire"
)

var (
	// ErrTokenRejected means the chat refused the bot token, so the admin must
	// replace it.
	ErrTokenRejected = chatwire.ErrTokenRejected
	// ErrNotMember means the bot may not act in this channel.
	ErrNotMember = chatwire.ErrNotMember
	// ErrChannelGone means the channel or message no longer exists.
	ErrChannelGone = chatwire.ErrChannelGone
	// ErrUnreachable means the chat could not be reached or answered with a fault.
	ErrUnreachable = chatwire.ErrUnreachable
	// ErrRequestRejected means the chat refused the request as malformed, or as
	// one it will not do.
	ErrRequestRejected = chatwire.ErrRequestRejected
	// ErrNameRefused means the chat will not make a channel by this name: it is
	// taken, or breaks the chat's naming rules.
	ErrNameRefused = chatwire.ErrNameRefused
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
