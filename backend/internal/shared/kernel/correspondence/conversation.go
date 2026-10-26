// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package correspondence

import "fmt"

// The conversation a message belongs to, and which of its rows are proven to
// be in it. The thread key is the sender's References root, so the key alone
// proves nothing. Capture, the deal answer and the request settlement all ask
// these questions, and they live here so every module asks them the same way.
// Aliases are compile-time SQL aliases, never request input.

// ConversationSQL is the test that row is in anchor's conversation: the same
// thread, medium and correspondent, and row is live. A null or empty thread
// key joins nothing, so two unthreaded mails are never one conversation.
func ConversationSQL(row, anchor string) string {
	return ThreadSQL(row, anchor) + fmt.Sprintf(`
     AND %[1]s.counterparty_email = %[2]s.counterparty_email`, row, anchor)
}

// ThreadSQL is the thread, medium and liveness half of ConversationSQL. It
// names no correspondent, so a caller must add one.
func ThreadSQL(row, anchor string) string {
	return fmt.Sprintf(`%[2]s.thread_key <> '' AND %[1]s.thread_key = %[2]s.thread_key
     AND %[1]s.kind = %[2]s.kind
     AND %[1]s.channel_provider IS NOT DISTINCT FROM %[2]s.channel_provider
     AND %[1]s.archived_at IS NULL`, row, anchor)
}

// OurOutboundSQL is the test that row is mail we sent. The direction alone is
// not: the provider's sent attestation or a colleague's sender stamp proves it.
func OurOutboundSQL(row string) string {
	return fmt.Sprintf(`%[1]s.direction = 'outbound'
     AND (%[1]s.counterparty_outbound_attested OR %[1]s.sent_by_colleague IS NOT NULL)`, row)
}

// AuthenticatedInboundSQL is the test that row is captured inbound mail whose
// sender the receiving server authenticated.
func AuthenticatedInboundSQL(row string) string {
	return fmt.Sprintf("(%[1]s.direction = 'inbound' AND %[1]s.captured_by LIKE 'connector:%%' AND %[1]s.inbound_sender_authenticated)", row)
}

// ProvenSQL is the test that row's place in its conversation cannot be
// forged: we sent it, or its sender was authenticated.
func ProvenSQL(row string) string {
	return `((` + OurOutboundSQL(row) + `) OR ` + AuthenticatedInboundSQL(row) + `)`
}
