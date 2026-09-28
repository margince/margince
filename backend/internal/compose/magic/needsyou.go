// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package magic

// The needs-you lane: a decision waiting.
//
// The same staged approvals /attention serves, drawn beside the rest of the
// machinery's output so a morning is one page rather than two. Still a router
// and not a second inbox: the line says a decision is waiting and names what it
// is about, and the approvals surface keeps the verb.

import (
	"context"
	"errors"
	"strings"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// PendingDecisions answers the approvals staged and undecided.
//
// A seam for the reason TroubledRuns is one, and OPTIONAL for the same reason:
// unbound is an empty lane, never a refusal of the page.
type PendingDecisions interface {
	PendingApprovals(ctx context.Context, limit int) ([]crmcontracts.Approval, error)
}

// WithPendingDecisions binds the needs-you lane.
func (s *Service) WithPendingDecisions(p PendingDecisions) *Service {
	s.pending = p
	return s
}

// needsYou reads the decisions waiting on a human.
//
// A REFUSED read is named rather than folded into an empty lane: "you may not
// see the queue" and "nothing is waiting" are opposite answers, and a reader
// told the second on the day they lost the grant would decide nothing and think
// they were done.
func (s *Service) needsYou(
	ctx context.Context, limit int,
) ([]crmcontracts.MagicLine, *crmcontracts.WorklistSourceUnavailable, error) {
	if s.pending == nil {
		return []crmcontracts.MagicLine{}, nil, nil
	}
	approvals, err := s.pending.PendingApprovals(ctx, limit)
	if err != nil {
		if errors.Is(err, apperrors.ErrPermissionDenied) {
			return []crmcontracts.MagicLine{}, &crmcontracts.WorklistSourceUnavailable{
				Source: sourceStagedApproval,
				Reason: crmcontracts.WorklistSourceUnavailableReasonWithheld,
			}, nil
		}
		return nil, nil, err
	}
	lines := make([]crmcontracts.MagicLine, 0, len(approvals))
	for _, approval := range approvals {
		line, ok := pendingLine(approval)
		if !ok {
			continue
		}
		lines = append(lines, line)
	}
	return lines, nil, nil
}

// sourceStagedApproval names this lane where a refusal is reported, in the
// worklist's own source vocabulary so a client that already draws those names
// needs no second one.
const sourceStagedApproval = "approval"

// approvalSentences is what each staged kind asks the reader about.
var approvalSentences = map[string]string{
	"coldstart":           "magic.action.approval_coldstart",
	"send_email":          "magic.action.approval_send_email",
	"advance_deal":        "magic.action.approval_advance_deal",
	"promote_lead":        "magic.action.approval_promote_lead",
	"overnight":           "magic.action.approval_overnight",
	"transcript_proposal": "magic.action.approval_transcript_proposal",
}

// sentenceForKind answers what one proposal asks about, and answers for a kind
// it does not recognise too.
//
// SHOWN UNDER A GENERIC SENTENCE, inverting this package's drop-the-unknown
// rule, and the asymmetry is the whole reason: a done line dropped costs a
// reader a change they already have, while a needs-you line dropped costs them
// a decision they never learn is waiting.
func sentenceForKind(kind string) string {
	if key, ok := approvalSentences[kind]; ok {
		return key
	}
	return "magic.action.approval_pending"
}

// pendingLine dresses one staged decision, or refuses it.
//
// TWO ROWS ARE REFUSED, and neither is lost by it: both stay on the approvals
// inbox, which is where a decision is answered and which this lane only
// mirrors.
//
// A row whose effective status is no longer pending. Expiry is LAZY — the
// column still reads pending until the sweep runs, and the engine folds the
// deadline in on the way out — so drawing the raw column would count a dead
// proposal in the reader's total and send them to a decision the write refuses.
//
// And a row a HUMAN staged. proposed_by carries the staging principal verbatim,
// and a rep asking for a consent override stages under their own name; this
// surface reports what ran WITHOUT being asked, which is why its actor
// vocabulary has no human member. Reporting a colleague's request back as
// machinery is the spookiness this package exists to remove, inverted.
func pendingLine(a crmcontracts.Approval) (crmcontracts.MagicLine, bool) {
	if a.Status != crmcontracts.ApprovalStatusPending {
		return crmcontracts.MagicLine{}, false
	}
	actor, machine := machineProposer(a.ProposedBy)
	if !machine {
		return crmcontracts.MagicLine{}, false
	}
	values := map[string]string{"kind": a.Kind}
	if a.TargetLabel != nil {
		// The caption frozen at staging time, which is what the approver was
		// shown; re-resolving it here would name whatever the record became.
		values["target"] = *a.TargetLabel
	}
	consequence := "magic.consequence.awaits_your_decision"
	line := crmcontracts.MagicLine{
		Id:         a.Id,
		OccurredAt: a.CreatedAt,
		Lane:       crmcontracts.MagicLineLaneMagicLaneNeedsYou,
		Summary: crmcontracts.MagicSentence{
			Key:    sentenceForKind(a.Kind),
			Values: &values,
		},
		Consequence: &consequence,
		Actor:       actor,
		// Nothing has happened yet, so there is nothing to put back. Stated
		// rather than absent, which a client would have to guess about.
		Undo: &crmcontracts.MagicUndo{Undoable: false, Reason: &nothingToUndo},
	}
	// Both halves or neither: an entity reference carrying a type with no id
	// points a reader at nothing.
	if a.TargetEntityType != nil && a.TargetEntityId != nil {
		line.Entity = &crmcontracts.MagicEntityRef{
			Type: *a.TargetEntityType,
			Id:   *a.TargetEntityId,
		}
	}
	if a.OnBehalfOf != nil {
		line.Actor.OnBehalfOf = a.OnBehalfOf
	}
	return line, true
}

// systemProposer is how the product names itself when no namespaced job did.
const systemProposer = "system"

// machineProposer reads who staged the proposal, and answers whether it was a
// machine at all.
//
// FAIL CLOSED ON PROVENANCE. An id this build cannot place is refused rather
// than filed under the system actor, because the likeliest thing it is, is a
// human: every principal id here is `agent:`, `connector:`, `system` or
// `human:`, and a default sweeping the remainder into `system` would launder
// exactly the case the contract forbids. An unplaceable proposer is also the
// shape a NEW principal kind arrives in, and guessing it is the product would
// be a claim this package cannot support.
func machineProposer(proposedBy string) (crmcontracts.MagicActor, bool) {
	if proposedBy == systemProposer {
		return crmcontracts.MagicActor{
			Type: crmcontracts.MagicActorTypeMagicActorSystem,
			Id:   proposedBy,
		}, true
	}
	prefix, id, found := strings.Cut(proposedBy, ":")
	if !found || id == "" {
		return crmcontracts.MagicActor{}, false
	}
	switch crmcontracts.MagicActorType(prefix) {
	case crmcontracts.MagicActorTypeMagicActorAgent,
		crmcontracts.MagicActorTypeMagicActorSystem,
		crmcontracts.MagicActorTypeMagicActorConnector:
		return crmcontracts.MagicActor{
			Type: crmcontracts.MagicActorType(prefix),
			Id:   id,
		}, true
	}
	return crmcontracts.MagicActor{}, false
}
