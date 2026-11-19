// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package heldreason names the hold reasons a communication review can stand
// over. activities holds the message and consent decides about it, and both
// read this list.
package heldreason

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
