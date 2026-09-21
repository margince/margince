// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import "fmt"

// ourOutboundInThisThread renders the clause set that identifies OUR OWN
// message in the same conversation as another one.
//
// Four statements asked this question and spelled it four times: the settlement
// pass's eligibility test, its watermark subselect, the same subselect again in
// the candidate read, and the owed-verdict pass's prior-message join. All four
// agreed on every clause here and differed only in what they added — which
// direction of the conversation they look in, what else the row must be, and
// how much of it they take. AGENTS.md asks for a shared helper or a reason;
// this is the helper, and each caller says beside itself what it adds.
//
// A THREAD KEY IS NOT EVIDENCE, which is why two of these clauses exist. The
// key is the message's own References root, so the SENDER types it: a stranger
// who has seen one of our Message-IDs — they were copied on the thread, it went
// to a list, somebody forwarded it — can send a cold mail carrying that root
// and manufacture a conversation out of correspondence we had with somebody
// else. Matching outbound on the key alone then reads our reply to THEM as a
// reply to the stranger. `counterparty_email` binds both halves to one
// correspondent and `counterparty_outbound_attested` is the provider's own
// filing of the message as sent there; a header can forge neither. capture's
// wroteBackTx refuses the same forgery on the same column.
//
// PLAIN EQUALITY on the key, never IS NOT DISTINCT FROM. NULL-matching would
// join every threadless row to every other, so one unthreaded outbound of ours
// would answer for every unthreaded conversation in the workspace. The
// provider is the opposite case and is compared with IS NOT DISTINCT FROM: a
// mail carries none, and two mails both carrying none are the same transport.
//
// It renders no placeholder, so a caller's own `$N` numbering is untouched by
// where this sits in the statement.
func ourOutboundInThisThread(row, anchor string) string {
	return fmt.Sprintf(`%[1]s.thread_key = %[2]s.thread_key
     AND %[1]s.kind = %[2]s.kind
     AND %[1]s.channel_provider IS NOT DISTINCT FROM %[2]s.channel_provider
     AND %[1]s.direction = 'outbound'
     AND %[1]s.counterparty_email = %[2]s.counterparty_email
     AND %[1]s.counterparty_outbound_attested
     AND %[1]s.archived_at IS NULL`, row, anchor)
}
