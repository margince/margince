// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

import (
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// sendShapedGrants is what deciding a kind that sends a message takes. It is
// the send's write, read of the conversation the draft came from, and read of
// the counterparty it is addressed to.
//
// The composer runs as the system principal, so the card can disclose both.
// The human's own grants decide who may see it. A class, not a kind, so a new
// send-shaped kind adds itself here.
var sendShapedGrants = []DecisionGrant{
	{objectActivity, principal.ActionCreate},
	{objectActivity, principal.ActionRead},
	{tableContact, principal.ActionRead},
}
