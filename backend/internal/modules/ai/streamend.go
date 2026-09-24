// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"fmt"

	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// streamEnd is what one stream has learned about how its reply ended, and how
// every adapter's Next turns that into the port's TokenStream terminal. Five
// wires spell their terminal five ways; what each answer means to a caller
// must not differ by wire.
//
// The terminal is recorded rather than returned at once because a wire may put
// the last of the text in the same event that ends the reply, and that text
// was generated and billed: Next delivers it, and the terminal answers the call
// after.
type streamEnd struct {
	wire string
	// read is set once the provider's own terminal has been parsed. A body
	// that closes before it is a dropped connection, never a finished answer.
	read bool
	// cutOff records a terminal that says the output ceiling stopped the reply.
	cutOff bool
}

// finish records the provider's terminal, in the port's vocabulary.
func (e *streamEnd) finish(reason string) {
	e.read = true
	e.cutOff = reason == model.FinishReasonLength
}

// outcome is Next's answer once there is no chunk left to deliver. scanErr is
// the stream reader's own failure, read only when no terminal arrived: after
// one, the reply is whole and a later read error describes nothing in it.
func (e *streamEnd) outcome(scanErr error) (string, bool, error) {
	switch {
	case e.cutOff:
		return "", false, truncatedError{wire: e.wire}
	case e.read:
		return "", false, nil
	case scanErr != nil:
		return "", false, fmt.Errorf("ai: %s: stream: %w", e.wire, scanErr)
	default:
		return "", false, fmt.Errorf("ai: %s: stream ended without a terminal event", e.wire)
	}
}

// sseErrorEvent is the event type Anthropic's and OpenAI's streams send for a
// failure after the 200 went out: an overload or a server fault mid-reply.
const sseErrorEvent = "error"
