// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mailidentity

import "testing"

func TestHasMachineLocalpartReadsOnlyTheLocalPart(t *testing.T) {
	t.Parallel()
	for address, want := range map[string]bool{
		"signing-noreply@example.com":  true,
		"mailer-daemon@example.com":    true,
		"receipts@example.com":         true,
		"Calendar-Notification@x.test": true,
		// A person whose name holds a marker word in the middle.
		"anna.notify.weber@example.com": false,
		// A mail-infrastructure domain is IsMachineAddress's question, not this one's.
		"anna@docusign.net": false,
		"noreply":           false,
		"@example.com":      false,
	} {
		if got := HasMachineLocalpart(address); got != want {
			t.Errorf("HasMachineLocalpart(%q) = %v, want %v", address, got, want)
		}
	}
}

func TestIsMachineAddressStillRefusesTheInfrastructureDomains(t *testing.T) {
	t.Parallel()
	if !IsMachineAddress("anna@docusign.net") {
		t.Error("an address on a transactional relay domain is no longer a machine address")
	}
	if !IsMachineAddress("noreply@example.com") {
		t.Error("a noreply local part is no longer a machine address")
	}
	if IsMachineAddress("anna@example.com") {
		t.Error("a person's address reads as a machine address")
	}
}
