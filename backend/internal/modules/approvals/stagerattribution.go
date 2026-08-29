// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

// Who may stage a proposal, and what a NULL passport_id is allowed to mean.

import (
	"context"
	"errors"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// stagerIsAttributable is the check every staging ENTRY POINT makes, and the
// entry is the boundary rather than the insert.
//
// A staging does not always insert. The joining path hands back a live row
// under the same identity, and StageAgentCall hands back an approved-and-unspent
// one — so a passport-less agent repeating a call that already has a
// NULL-passport approval would be given that row without an insert happening at
// all, and the redemption skips the passport binding for a caller presenting
// none. Guarded at the insert alone, the second attempt walks through.
func stagerIsAttributable(ctx context.Context) error {
	p, ok := principal.Actor(ctx)
	if !ok {
		return errors.New("crmapprovals: no actor bound to context")
	}
	return attributableStager(p)
}

// attributableStager refuses an AGENT staging without its passport.
//
// A zero PassportID stores NULL, and a NULL passport_id is what a SERVER
// proposal looks like — the case agentMayDecide deliberately admits, since a row
// nobody's credential made has no self-approval to refuse. So an agent row
// staged without one reads as proposed by nobody: the refusal compares the two
// passports and never fires, and the credential can release the call it just
// made. That loop is the whole of what the confirm-first tier exists to stop.
//
// Only an agent. A SYSTEM principal legitimately carries no passport — the
// nightly sweeps and the site reads propose changes on nobody's credential —
// and the server-proposed case has to stay reachable or this would break the
// path it is protecting.
//
// Refused at the STAGING BOUNDARY rather than left to callers, so what a NULL
// passport_id MEANS becomes a property of the row instead of a property of every
// writer having been careful — which is a statement about today's callers, not
// about the column. No agent SURFACE builds such a principal today, which is why
// this is a guard rather than a repair: AgentIdentity.Principal always carries
// the passport it authenticated. compose/autoapply.go does build one, and it
// only ever decides — the sweep releases under the owner's own policy, which is
// not a credential releasing its own proposal.
func attributableStager(p principal.Principal) error {
	if p.Type != principal.PrincipalAgent || p.PassportID != ids.Nil {
		return nil
	}
	return errors.New("crmapprovals: an agent stages on its passport — a proposal carrying none is " +
		"indistinguishable from one the server made, and the credential could then release it")
}
