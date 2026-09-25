// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

import (
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// sameAgent answers whether the caller IS the agent that staged this proposal.
//
// Three rules rest on that question and none of them means "the same passport
// row". A refresh spends the presented token, retires the passport and mints a
// replacement under the same grant (identity/oauth_refresh.go), so an agent that
// does nothing but keep its client running answers to a new passport id every
// access-token lifetime, with the same connection, the same lender and the same
// scopes.
//
// The connection is asked first because it is the durable half. Passport
// equality still answers for a passport a human minted directly: those carry no
// grant, and minting one is human-only in the contract — held by
// TestEveryHumanOnlyOperationReachesTheGate, which derives its corpus from
// x-agent-access — so the row id is the only identity such a credential has, and
// getting a second one costs an agent a human session it cannot open.
//
// Held by: TestTheProposersIdentityHasOneSpelling (backend/gates/approvalsameagent_test.go)
func sameAgent(a row, p principal.Principal) bool {
	if a.ConnectionID != nil && p.ConnectionID != ids.Nil && *a.ConnectionID == p.ConnectionID {
		return true
	}
	return a.PassportID != nil && p.PassportID != ids.Nil &&
		a.PassportID.UUID == p.PassportID
}
