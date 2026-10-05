// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package statedreason is what counts as a written reason for a decision that
// narrows what the installation keeps: present once trimmed, and no longer than
// the contract's RetentionOverrideRequest allows. The controller's overrides and
// the undo of a project filing decode that one request body, so they share one
// answer here; a gate ties Max to the contract's maxLength.
package statedreason

import "strings"

// Max is RetentionOverrideRequest.reason's maxLength, in characters.
const Max = 2000

// Trim answers the reason as stated and whether it is admissible.
//
// The bound is checked on the reason as sent, before trimming, because that is
// what the contract's maxLength binds: edge whitespace does not buy extra room.
func Trim(reason string) (string, bool) {
	if len([]rune(reason)) > Max {
		return "", false
	}
	stated := strings.TrimSpace(reason)
	return stated, stated != ""
}
