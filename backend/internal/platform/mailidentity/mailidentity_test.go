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
		// A human whose name holds a marker word in the middle.
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
		t.Error("a human's address reads as a machine address")
	}
}

// Bounces do not arrive with a tidy localpart. VERP encodes the original
// recipient into the address and BATV signs the return path, so the shapes a
// real mail system emits carry tags a plain equality check never sees.
func TestDeliverySystemSendersAreRecognizedInTheShapesTheyArriveIn(t *testing.T) {
	drops := []string{
		"MAILER-DAEMON@x.com", "m.a.i.l.e.r-daemon@x.com", "post-master@x.com",
		"mailer_daemon@x.com", // `_` too — the T2 registry strips it, so this must
		"bounces-12345@x.com", "bounce+tag@x.com",
		"bounce+tag-42@x.com", "prvs=1234abcd=owner@x.com", "msprvs1=abc=bounces@x.com",
	}
	for _, addr := range drops {
		if !IsDeliverySystemSender(addr) {
			t.Errorf("%s reached the tier gate — there is no correspondent behind the transport system", addr)
		}
	}
	keeps := []string{"no-reply@x.com", "notifications@x.com", "alice@x.com", "bouncer@x.com", "postmaster.team@x.com", "postmaster", ""}
	for _, addr := range keeps {
		if IsDeliverySystemSender(addr) {
			t.Errorf("%s was dropped as delivery-system mail — a real sender is behind it", addr)
		}
	}
}
