// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package documenthost names the hosts that serve shared documents: Google
// Docs, Sheets, Slides, Forms and Drive. Capture reads a mail's links to them
// as documents sent; the link preview leaves them unfetched, because they
// answer a sign-in page to a reader who is not signed in.
package documenthost

import "strings"

var hosts = map[string]struct{}{
	"docs.google.com":  {},
	"drive.google.com": {},
}

// Is reports whether host, lowercased, serves shared documents.
func Is(host string) bool {
	_, known := hosts[strings.TrimSuffix(host, ".")]
	return known
}
