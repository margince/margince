// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import "github.com/margince/margince/backend/internal/shared/kernel/correspondence"

// Provider attestation or a colleague's retained copy establishes that an outbound is ours.
// A sender-controlled thread key alone cannot prove that we answered this correspondent.
func ourOutboundInThisThread(row, anchor string) string {
	return correspondentInThisThread(row, anchor) + `
     AND ` + correspondence.OurOutboundSQL(row)
}

// CapturedInboundSQL requires both connector provenance and receiving-server sender authentication.
func CapturedInboundSQL(alias string) string {
	return correspondence.AuthenticatedInboundSQL(alias)
}

// Null thread keys must not join unrelated mail; a missing provider still identifies email.
func correspondentInThisThread(row, anchor string) string {
	return correspondence.ConversationSQL(row, anchor)
}
