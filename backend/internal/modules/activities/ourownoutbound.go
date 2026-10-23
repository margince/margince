// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"fmt"

	"github.com/margince/margince/backend/internal/shared/kernel/correspondence"
)

// Provider attestation or a colleague's retained copy establishes that an outbound is ours.
// A sender-controlled thread key alone cannot prove that we answered this correspondent.
func ourOutboundInThisThread(row, anchor string) string {
	return correspondentInThisThread(row, anchor) + `
     AND ` + correspondence.OurOutboundSQL(row)
}

// CapturedInboundSQL requires both connector provenance and receiving-server sender authentication.
func CapturedInboundSQL(alias string) string {
	return fmt.Sprintf("(%[1]s.direction = 'inbound' AND %[1]s.captured_by LIKE 'connector:%%' AND %[1]s.inbound_sender_authenticated)", alias)
}

// Null thread keys must not join unrelated mail; a missing provider still identifies email.
func correspondentInThisThread(row, anchor string) string {
	return fmt.Sprintf(`%[1]s.thread_key = %[2]s.thread_key
     AND %[1]s.kind = %[2]s.kind
     AND %[1]s.channel_provider IS NOT DISTINCT FROM %[2]s.channel_provider
     AND %[1]s.counterparty_email = %[2]s.counterparty_email
     AND %[1]s.archived_at IS NULL`, row, anchor)
}
