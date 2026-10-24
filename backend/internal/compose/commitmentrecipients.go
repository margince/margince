// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const commitmentRecipientsPrompt = `"recipient_refs" lists the distinct verified contact references to whom our colleague made this exact promise. Include several only when the cited words promise the same deliverable to all of them. Being copied or attending alone does not establish a recipient. Use [] when no recipient is established, the promise is not ours, or the event is not a commitment. Never combine different asks or invent references. `

const commitmentRecipientsKey = "recipient_refs"

func validateCommitmentRecipients(refs []string, parties []commitmentParty) string {
	seen := map[string]bool{}
	for _, ref := range refs {
		party, found := referencedPromiser(ref, parties)
		if !found || party.Contact == nil || seen[ref] {
			return "a promise's recipients must be distinct verified contact participants"
		}
		seen[ref] = true
	}
	return ""
}

// An explicit empty list means attribution is unknown; omitted legacy output keeps its single recipient.
func attributeCommitmentRecipients(c *Commitment, refs []string, parties []commitmentParty) error {
	if msg := validateCommitmentRecipients(refs, parties); msg != "" {
		return fmt.Errorf("recipient attribution changed: %w", apperrors.ErrVersionSkew)
	}
	if refs == nil || c.Theirs != nil {
		return nil
	}
	c.PromisedTo = nil
	for _, ref := range refs {
		party, found := referencedPromiser(ref, parties)
		if !found || party.Contact == nil {
			continue
		}
		recipient := ids.From[ids.ContactKind](*party.Contact)
		if !slices.Contains(c.Recipients, recipient) {
			c.Recipients = append(c.Recipients, recipient)
		}
	}
	slices.SortFunc(c.Recipients, func(a, b ids.ContactID) int { return strings.Compare(a.String(), b.String()) })
	if len(c.Recipients) == 1 {
		c.PromisedTo = &c.Recipients[0]
	}
	return nil
}

func commitmentRecipients(c Commitment) []ids.ContactID {
	recipients := slices.Clone(c.Recipients)
	if c.PromisedTo != nil && !slices.Contains(recipients, *c.PromisedTo) {
		recipients = append(recipients, *c.PromisedTo)
	}
	return recipients
}

func (d *CommitmentDispatcher) fileRecipientClaims(ctx context.Context, tx pgx.Tx, c Commitment, reason activities.CommitmentAdmissionReason, unlinkedOnly bool) ([]ids.UUID, bool, error) {
	// Recipient reconciliation must not renew a deadline before its task renewal is admitted.
	c.AdmissionSourceID = nil
	recipients := commitmentRecipients(c)
	claims := make([]ids.UUID, 0, len(recipients))
	open := len(recipients) == 0
	for _, recipient := range recipients {
		claim, _, err := d.fileClaim(ctx, tx, c, recipient, claimKindOurs, reason)
		if err != nil {
			return nil, false, err
		}
		if unlinkedOnly && (claim.TaskActivityId != nil || claim.Status != crmcontracts.ConversationClaimStatusOpen) {
			continue
		}
		claims = append(claims, ids.UUID(claim.Id))
		open = open || claim.Status == crmcontracts.ConversationClaimStatusOpen
	}
	return claims, open, nil
}

func (d *CommitmentDispatcher) reconcileTaskRecipients(ctx context.Context, tx pgx.Tx, c Commitment, reason activities.CommitmentAdmissionReason, bundleID ids.UUID) (CommitmentOutcome, ids.UUID, error) {
	if !canReconcileTaskRecipients(c, reason) {
		return CommitmentRemembered, ids.Nil, nil
	}
	if c.AdmissionSourceID != nil {
		held, _, err := activities.CommitmentAdmissionStateTx(ctx, tx, ids.From[ids.ActivityKind](c.SourceActivityID), c.Locator, c.AsOf, c.AdmissionSourceID, c.RenewedAt)
		if err != nil || held {
			return CommitmentRemembered, ids.Nil, err
		}
	}
	target, mutable, err := d.recipientTaskTarget(ctx, tx, c)
	if err != nil || target.ID.IsZero() {
		return CommitmentRemembered, ids.Nil, err
	}
	claims, _, err := d.fileRecipientClaims(ctx, tx, c, reason, true)
	if err != nil || len(claims) == 0 {
		return CommitmentRemembered, ids.Nil, err
	}
	if mutable.IsZero() {
		proposal := CommitmentTaskProposal{SourceActivityID: c.SourceActivityID, Summary: c.Summary, Party: c.Party, Quote: c.Quote, Locator: c.Locator, SeatID: target.Assignee, PromiserSeatID: c.Seat, ClaimIDs: claims, Body: c.Body, PrivateTo: c.PrivateTo, ExistingTaskID: &target.ID}
		asked, _, err := d.pendingProposalAskedOf(ctx, tx, c, &proposal)
		if err != nil {
			return CommitmentRemembered, ids.Nil, err
		}
		return d.stageCommitmentProposal(ctx, tx, c, proposal, bundleID, asked, nil)
	}
	for _, claim := range claims {
		if err := d.claims.SetClaimTaskTx(ctx, tx, claim, target.ID); err != nil {
			return CommitmentRemembered, ids.Nil, err
		}
	}
	return CommitmentRemembered, ids.Nil, nil
}

func (d *CommitmentDispatcher) reconcilePendingRecipients(ctx context.Context, tx pgx.Tx, c Commitment, held approvals.Offer, bundleID ids.UUID, late bool) (CommitmentOutcome, ids.UUID, error) {
	if !held.Pending() || c.Theirs != nil || c.Meaning.withheldOutcome() != "" || len(commitmentRecipients(c)) == 0 {
		return CommitmentRemembered, ids.Nil, nil
	}
	var proposal CommitmentTaskProposal
	if err := json.Unmarshal(held.Change, &proposal); err != nil {
		return CommitmentRemembered, ids.Nil, fmt.Errorf("read pending commitment recipients: %w", err)
	}
	if proposal.Locator != c.Locator {
		return CommitmentRemembered, ids.Nil, nil
	}
	claims, _, err := d.fileRecipientClaims(extractorContext(ctx, c.Extractor), tx, c, activities.CommitmentCurrent, proposal.ExistingTaskID != nil)
	if err != nil {
		return CommitmentRemembered, ids.Nil, err
	}
	if !proposal.addUnlistedClaims(claims) {
		return CommitmentRemembered, ids.Nil, nil
	}
	asked, state, err := d.pendingProposalAskedOf(ctx, tx, c, &proposal)
	if err != nil {
		return CommitmentRemembered, ids.Nil, err
	}
	// Supersession matches only the new decider's rows, so a request still put
	// to a colleague the promise no longer names is withdrawn here.
	if held.Decider != nil && *held.Decider != asked {
		if _, err := d.approval.WithdrawInTx(ctx, tx, held.ID, "the promise's task now belongs to another colleague"); err != nil {
			return CommitmentRemembered, ids.Nil, err
		}
	}
	return d.stageCommitmentProposal(ctx, tx, c, proposal, bundleID, asked, commitmentWindow(state, late))
}

// addUnlistedClaims appends the claims the proposal does not name yet and
// reports whether it added any.
func (p *CommitmentTaskProposal) addUnlistedClaims(claims []ids.UUID) bool {
	changed := false
	for _, claim := range claims {
		if (p.ClaimID != nil && *p.ClaimID == claim) || slices.Contains(p.ClaimIDs, claim) {
			continue
		}
		p.ClaimIDs = append(p.ClaimIDs, claim)
		changed = true
	}
	return changed
}

func (d *CommitmentDispatcher) recipientTaskTarget(ctx context.Context, tx pgx.Tx, c Commitment) (activities.CommitmentTaskTarget, ids.UUID, error) {
	target, err := d.tasks.OpenCommitmentTaskTx(ctx, tx, c.Locator, c.PrivateTo)
	if refused(err) {
		return activities.CommitmentTaskTarget{}, ids.Nil, nil
	}
	if err != nil || target.ID.IsZero() {
		return activities.CommitmentTaskTarget{}, ids.Nil, err
	}
	mutable, err := d.tasks.MutableCommitmentTaskTx(ctx, tx, c.Locator, c.PrivateTo)
	if refused(err) {
		return activities.CommitmentTaskTarget{}, ids.Nil, nil
	}
	if err != nil {
		return activities.CommitmentTaskTarget{}, ids.Nil, err
	}
	return target, mutable, nil
}

func canReconcileTaskRecipients(c Commitment, reason activities.CommitmentAdmissionReason) bool {
	return reason == activities.CommitmentCurrent && c.Meaning.withheldOutcome() == "" && !c.NeedsConfirmation && len(commitmentRecipients(c)) > 0
}
