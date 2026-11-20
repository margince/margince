// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/correspondence"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
)

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

// splitAnswersSQL selects our reply on a new topic to request a, by the same
// tests as our reply in its conversation. Its key is its own, so its delivery
// names the message it answers.
func splitAnswersSQL(private bool) string {
	return `SELECT reply.id, reply.occurred_at FROM ` + storekit.SplitRepliesSQL("reply", "a") + `
     AND reply.kind = a.kind AND reply.archived_at IS NULL
     AND reply.counterparty_email = a.counterparty_email
     AND ` + correspondence.OurOutboundSQL("reply") + `
     AND ` + judgeableAudienceSQL("reply", private) + `
     AND ` + occurredday.BegunSQL("reply", "$1") + `
     AND ` + occurredday.AnswersSQL("reply", "a")
}
