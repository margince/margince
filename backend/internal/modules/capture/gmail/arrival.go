// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gmail

import (
	"strconv"
	"time"
)

// internalDate reads Gmail's internalDate: when the message reached this
// mailbox, by Google's clock, sent as epoch milliseconds in a string. It rides
// the messages.get response and is not read from any header, so a sender
// cannot backdate it the way they can write any Date line.
//
// Anything that does not parse as a positive number is no time at all: a zero
// time claims nothing, where a guessed one would.
func internalDate(ms string) time.Time {
	n, err := strconv.ParseInt(ms, 10, 64)
	if err != nil || n <= 0 {
		return time.Time{}
	}
	return time.UnixMilli(n).UTC()
}
