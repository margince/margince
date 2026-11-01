// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import "slices"

// MailProviders names the mailbox providers offered by connection settings.
func MailProviders() []string { return []string{providerGmail, providerGraph, "imap"} }

// IsMailProvider distinguishes mailbox capabilities from calendars.
func IsMailProvider(provider string) bool {
	return slices.Contains(MailProviders(), provider)
}

// DeliversMail includes the test mailbox without offering it in account settings.
func DeliversMail(provider string) bool {
	return IsMailProvider(provider) || provider == "test_mailbox"
}
