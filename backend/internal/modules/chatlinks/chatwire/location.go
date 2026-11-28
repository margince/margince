// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatwire

import "context"

// MessageLocation binds a selected message to its canonical thread root.
type MessageLocation struct {
	Message string
	Root    string
}

// MessageLocator resolves provider references before explicit message filing.
type MessageLocator interface {
	LocateMessage(context.Context, string, string) (MessageLocation, error)
}
