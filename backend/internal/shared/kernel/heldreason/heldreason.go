// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package heldreason names the hold reasons a communication review can stand
// over. activities holds the message and consent decides about it, and both
// read this list.
package heldreason

import "errors"

const (
	// SendRefused is a message an immediate send froze when refused.
	SendRefused = "send_refused"
	// ConsentWithdrawn is a scheduled message the engine refused at fire.
	ConsentWithdrawn = "consent_withdrawn"
)

// Reviewable are the hold reasons a review's message may carry.
func Reviewable() []string {
	return []string{SendRefused, ConsentWithdrawn}
}

// Holder is an error that names the hold reason it stops a scheduled message
// with. The module holding the message then needs no import of the module that
// raised the error.
type Holder interface {
	error
	HeldReason() string
}

// Of answers the hold reason an error carries.
func Of(err error) (string, bool) {
	if holder, ok := errors.AsType[Holder](err); ok {
		return holder.HeldReason(), true
	}
	return "", false
}
