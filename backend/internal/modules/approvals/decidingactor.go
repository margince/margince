// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Who is behind a decision, and what their credential may spend on it.
//
// authority.go answers whether an approval is decidable AT ALL by the grants
// and row scope its release needs — the same question for a contact in their own
// seat and for an agent acting on their behalf. This file answers the other one,
// which only exists because a decision can now arrive on a lent credential: is
// there a contact behind this call, and did they lend it enough to release THIS.
//
// The split is the difference between authority and admission. A passport
// carries its human's grants, so nothing in authority.go needs to know it is a
// passport; what it cannot see is that a credential is a bounded loan, and a
// bound the lender set is not one the borrower may lift.

package approvals

import (
	"context"
	"fmt"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// actingForAHuman guards the inbox and the decision: a decision is a human's,
// whether it arrives from a browser or from a passport acting for one
// (platform/auth.RequireActingForAHuman states the rule). What a passport may RELEASE
// is agentMayDecide below; being somebody's agent is admission, not authority.
func actingForAHuman(ctx context.Context) error {
	return auth.RequireActingForAHuman(ctx, "approvals are decided")
}

// sendingKinds are the kinds whose APPROVAL puts a message on the wire at the
// moment of decision, rather than releasing a retry the caller performs itself.
//
// The distinction is what makes this list short, and it is the reason it cannot
// be derived from the kind's name. A staging an agent made is redeemed by that
// agent re-issuing its own call, which the admission gate re-admits against the
// passport's caps — so a send_email an agent staged is already bounded by `send`
// at the moment it is actually sent, and needs nothing here. These two are
// staged by the SERVER: approving them IS the send, and this is the only place
// a cap can bound it.
var sendingKinds = map[string]bool{
	// An automation-composed reply, held for its rep. redeem.go's own entry for
	// this kind says the release CREATES the outbound activity.
	kindHeldDraft: true,
	// A scheduled message the system stopped and is holding (ADR-0104 §5);
	// approving it lets it go.
	KindScheduledSendHeld: true,
}

// ReleaseSends reports whether APPROVING this kind puts a message on the wire at
// the moment of decision, which is the classification agentMayDecide charges the
// send cap on.
//
// Exported for the composition gate rather than for a caller: this module cannot
// see which effects the composition root registers, so it cannot tell that a new
// held-message kind has arrived — and the failure is silent, a credential whose
// human withheld `send` releasing a send. The census lives where both halves are
// visible (compose), and this is how it asks.
func ReleaseSends(kind string) bool { return sendingKinds[kind] }

// agentMayDecide bounds what a PASSPORT may do to one staged proposal, given
// that actingForAHuman has already admitted it as somebody's agent.
//
// A credential acts for its human with that human's permissions: what the human
// could release in the CRM, it may release from the conversation. The rules here
// are the places that sentence stops short — a release that cannot be taken
// back, a proposal staged for somebody else, and caps the human withheld.
//
// A human decides on the strength of their seat and their grants; an agent
// decides on the strength of a credential a human minted with a fixed set of
// caps, and "acting as the user" must not mean spending caps the user withheld.
// The caps a decision spends are the ones the release spends: `write`, because a
// decision is a durable change to somebody else's queue, and `send` on top of it
// where approving is the send.
//
// Only on approve. A rejection discards a proposal and cannot escalate, so an
// agent may always take a request off somebody's desk — its own included.
//
// A human principal carries no ScopeSet at all (scopes are a passport's shape,
// not a seat's), so this answers nothing for them and must not be asked.
func agentMayDecide(p principal.Principal, a row, approve bool, own ownRelease) error {
	if p.Type != principal.PrincipalAgent {
		return nil
	}
	if approve {
		if err := agentReleasesOnlyItsHumansProposal(p, a, own); err != nil {
			return err
		}
	}
	kind := a.Kind
	// A step-up is a question ABOUT this credential — how much of what it may
	// already read it may be handed (§2.4) — and it is the one decision no
	// on_behalf_of makes safe to delegate. The window exists because a human set
	// it; a passport that can lift its own window has none. Both verdicts, not
	// just the release: the lender is who this card was raised for, and an agent
	// answering it at all takes the question away from them.
	if kind == KindVolumeRelease {
		return fmt.Errorf("a volume step-up is answered by the human who lent this credential, not by it: %w",
			apperrors.ErrPermissionDenied)
	}
	if !p.Scopes.Has(principal.ScopeWrite) {
		return fmt.Errorf("deciding a staged action spends the write cap, which this credential does not carry: %w",
			apperrors.ErrPermissionDenied)
	}
	if approve && sendingKinds[kind] && !p.Scopes.Has(principal.ScopeSend) {
		return fmt.Errorf("approving a %s proposal sends the message it holds, which spends the send cap "+
			"this credential does not carry: %w", kind, apperrors.ErrPermissionDenied)
	}
	return nil
}

// agentReleasesOnlyItsHumansProposal is the approve-only half of agentMayDecide.
//
// AN AGENT DOES NOT CONFIRM ANOTHER CONTACT'S PROPOSAL. Two humans each lend a
// passport: A's stages the confirm-first call, B's approves it, A's redeems it,
// and the tier has been satisfied by two agents with nobody having looked.
// UserID is the comparison because a passport carries its lender's user id
// (AgentIdentity.Principal) — the same "is this your own business" test
// decidable() applies to a self-only kind.
//
// A row with NO recorded human is a SERVER proposal (attributableStager
// guarantees a NULL passport_id means that): the unattended policy apply,
// bounded by the owner's own authority rather than by a staging.
func agentReleasesOnlyItsHumansProposal(p principal.Principal, a row, own ownRelease) error {
	if a.OnBehalfOf != nil && (p.UserID == ids.Nil || a.OnBehalfOf.UUID != p.UserID) {
		return fmt.Errorf("this credential acts for somebody other than the human this action was "+
			"staged for, so it does not release it — that contact answers it themselves: %w",
			apperrors.ErrPermissionDenied)
	}
	if a.PassportID != nil && (own == ownReleaseRefused || a.OnBehalfOf == nil) {
		return agentReleasesNoIrreversibleProposal(p, a)
	}
	return nil
}

// agentReleasesNoIrreversibleProposal holds the agent-staged proposals whose
// release cannot be taken back — a message sent, a page fetched — for the human
// to release in the CRM. Asked of the agent that staged it, it stops the loop
// that needs nobody: stage, release, re-issue. Asked of a connected credential,
// it stops the same loop run on two credentials the human lent once each.
//
// sameAgent, not passport equality: a credential that rotates its token is the
// same agent afterwards. A credential the human minted by hand still releases
// another credential's proposal, because minting it took the human's session.
func agentReleasesNoIrreversibleProposal(p principal.Principal, a row) error {
	if sameAgent(a, p) {
		return fmt.Errorf("this credential proposed the action, and what it does cannot be taken back once "+
			"released — a sent message stays sent — so the contact it acts for releases it in the CRM: %w",
			apperrors.ErrPermissionDenied)
	}
	if p.ConnectionID != ids.Nil {
		return fmt.Errorf("a connected credential does not release an action another credential staged "+
			"when what it does cannot be taken back — the contact releases it in the CRM, or on a "+
			"credential they minted themselves: %w", apperrors.ErrPermissionDenied)
	}
	return nil
}
