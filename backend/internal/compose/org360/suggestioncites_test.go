// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package org360

import "testing"

// The origin on an outbound receipt names the channel, so "Email you sent"
// is a claim a reader can check against their own sent folder; a channel
// the rule has no words for still says who spoke.
func TestTheSentOriginNamesTheChannelWeSpokeOn(t *testing.T) {
	for kind, want := range map[string]string{
		"email":   "Email you sent",
		"message": "Message you sent",
		"call":    "Call you made",
		"meeting": "Meeting you held",
		"fax":     "Sent by you",
	} {
		if got := sentOrigin(kind); got != want {
			t.Errorf("sentOrigin(%q) = %q, want %q", kind, got, want)
		}
	}
}
