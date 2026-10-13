// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package connector

// SenderAuthentication is one receiving server's verdict on who sent a message.
// Server is the authserv-id the result names. DMARCPassDomain is the
// From domain DMARC passed for, empty when it did not pass.
type SenderAuthentication struct {
	Server          string
	DMARCPassDomain string
}
