// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package commsauthz

import (
	"regexp"
	"strings"
)

// NormalizedPurposeKey is how two purpose keys are compared: a key typed with
// a stray space or a capital names the same purpose.
func NormalizedPurposeKey(key string) string {
	return strings.TrimSpace(strings.ToLower(key))
}

// PurposeKeyPattern is what a new purpose's key may hold once normalised. Mail,
// forms and the consent screens name a purpose by it, so it holds no spaces.
// Its declared type keeps it out of this package's census of reason codes.
var PurposeKeyPattern *regexp.Regexp = regexp.MustCompile(`^[a-z0-9_]+$`)

// CanonicalPurpose is the one purpose key a message is judged, unsubscribed
// and stored under. A legacy key and a marketing purpose that name two
// different purposes are one message claiming two things, and it answers
// false. A reader of a message's purpose asks this, so a queued message and the
// engine name one purpose.
func (r Request) CanonicalPurpose() (Request, bool) {
	switch {
	case r.MarketingPurpose == "":
	case r.LegacyPurposeKey == "":
		r.LegacyPurposeKey = r.MarketingPurpose
	case NormalizedPurposeKey(r.LegacyPurposeKey) != NormalizedPurposeKey(r.MarketingPurpose):
		return r, false
	}
	return r, true
}
