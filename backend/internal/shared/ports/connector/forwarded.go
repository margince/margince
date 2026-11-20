// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package connector

// ForwardedOriginal is what a forwarded mail says about the original it carries:
// a message/rfc822 part, or an inline forward a mail program marked as one.
//
// Every field is text the forwarder could edit. It names a party the forward is
// about, never the forward's author. No reader may take it as proof of who
// wrote anything or of who may read anything.
type ForwardedOriginal struct {
	// From is the original's sender, a bare address in lower case.
	From        string
	DisplayName string
	Subject     string
	// MessageID is the original's own Message-ID, empty for an inline forward,
	// whose quoted header block carries none.
	MessageID string
	// Date is the original's date line as written, which no reader parses.
	Date string
	// Inline says the original's text is quoted in the forward's own body.
	Inline bool
}
