// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

// Who may stage a proposal, and what a NULL passport_id is allowed to mean.

import (
	"context"
	"errors"
	"fmt"

	"github.com/margince/margince/backend/internal/platform/approvalsubject"
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
//
// It also refuses a payload-owned proposal that names nobody it may be put to.
// The refusal comes before a join, supersession or declined-memory read can match it.
func stagerIsAttributable(ctx context.Context, in StageInput) error {
	p, ok := principal.Actor(ctx)
	if !ok {
		return errors.New("crmapprovals: no actor bound to context")
	}
	if err := attributableStager(p); err != nil {
		return err
	}
	return payloadNamesItsOwner(p, in)
}

// payloadNamesItsOwner refuses a payload-owned proposal that is for nobody, or
// that an agent stages for a member other than the one it is for. Either would
// put one colleague's promise or mail in front of another. A transfer and an
// edit access request go to the other party by design. An agent stages either
// for the colleague asking.
func payloadNamesItsOwner(p principal.Principal, in StageInput) error {
	if !approvalsubject.PayloadOwned(in.Kind) {
		return nil
	}
	owner := approvalsubject.Owner(in.Kind, in.ProposedChange)
	stagedFor := owner
	switch in.Kind {
	case approvalsubject.KindOwnershipTransfer:
		stagedFor = approvalsubject.OwnershipTransferRequester(in.ProposedChange)
	case approvalsubject.KindEditAccess:
		stagedFor = approvalsubject.EditAccessRequester(in.ProposedChange)
	}
	if owner.IsZero() || (!p.OnBehalfOf.IsZero() && p.OnBehalfOf != stagedFor) {
		return fmt.Errorf("crmapprovals: a %s proposal is staged only for the colleague it is for: %w", in.Kind, ErrNoDecider)
	}
	return nil
}

// attributableStager refuses an agent with neither a passport nor a person it
// acts for. Its row would name nobody and read as a server proposal, with no
// self-approval for agentMayDecide to refuse. The product's own job acting for
// a named person stages without a passport: its row names that person, and no
// credential exists to release it.
func attributableStager(p principal.Principal) error {
	if p.Type != principal.PrincipalAgent || p.PassportID != ids.Nil || p.OnBehalfOf != ids.Nil {
		return nil
	}
	return errors.New("crmapprovals: an agent stages on its passport or for a named person — a proposal " +
		"carrying neither is indistinguishable from one the server made, and the credential could then release it")
}
