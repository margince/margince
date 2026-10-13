// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import "testing"

// Only a transport whose provider filed the message may turn a From line into
// the seat's own sent mail: an IMAP server says whatever the seat's server says.
func TestOnlyAProviderFiledTransportVouchesForSentMail(t *testing.T) {
	for capturedBy, want := range map[string]bool{
		"connector:gmail":     true,
		"connector:graph":     true,
		"connector:imap":      false,
		"connector:extension": false,
		"":                    false,
	} {
		if got := providerFiledTransport(capturedBy); got != want {
			t.Errorf("providerFiledTransport(%q) = %v, want %v", capturedBy, got, want)
		}
	}
}
