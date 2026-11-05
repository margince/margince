// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package commsauthz

import "strings"

// NormalizedPurposeKey is how two purpose keys are compared: a key typed with
// a stray space or a capital names the same purpose.
func NormalizedPurposeKey(key string) string {
	return strings.TrimSpace(strings.ToLower(key))
}

// CanonicalPurpose rejects conflicting keys and fills the legacy key when
// only a marketing purpose was supplied.
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
