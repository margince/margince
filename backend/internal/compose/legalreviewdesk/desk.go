// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package legalreviewdesk tells the seats that may answer a legal review that a
// deal waits for one. Each gets an entry in their approvals inbox. The approval
// notice lane announces it in the app, and nothing is sent by email.
package legalreviewdesk

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/notices"
	"github.com/margince/margince/backend/internal/platform/approvalsubject"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

const (
	actor       = "system:legal_review_desk"
	summaryCode = "approval.legalReviewAsked"
	targetType  = "legal_review"
	objectName  = approvalsubject.ObjectLegalReview
	identityKey = "review_id"
	settledNote = "the legal review was answered or withdrawn"
)

// Desk implements deals.LegalReviewDesk over the approvals inbox.
type Desk struct {
	Approvals *approvals.Service
	Notices   *notices.Store
}

var _ deals.LegalReviewDesk = Desk{}

// Reviewers lists the live seats that may read and answer legal reviews.
func (Desk) Reviewers(ctx context.Context, tx pgx.Tx) ([]ids.UUID, error) {
	// The legal review object is a core one, so the core vocabulary decides.
	seats, err := identity.LiveSeatsAllowingTx(principal.SystemActing(ctx, actor), tx, identity.RbacObjects{},
		objectName, approvalsubject.LegalReviewDecisionActions()...)
	if err != nil {
		return nil, err
	}
	out := make([]ids.UUID, len(seats))
	for i, seat := range seats {
		out[i] = seat.UUID
	}
	return out, nil
}

// Raise stages one inbox entry per seat. A repeat joins the open entry.
func (d Desk) Raise(ctx context.Context, tx pgx.Tx, request deals.ReviewRequest) error {
	system := principal.SystemActing(ctx, actor)
	identity, err := json.Marshal(map[string]string{identityKey: request.ReviewID.String()})
	if err != nil {
		return fmt.Errorf("legalreviewdesk: encoding the identity: %w", err)
	}
	for _, seat := range request.Seats {
		body, err := json.Marshal(approvalsubject.LegalReviewRequest{
			ReviewID: request.ReviewID, DealID: request.DealID, DealName: request.DealName, OwnerID: seat,
		})
		if err != nil {
			return fmt.Errorf("legalreviewdesk: encoding the entry: %w", err)
		}
		digest := sha256.Sum256(body)
		_, err = d.Approvals.StageOrJoinPendingInTx(system, tx, approvals.StageInput{
			Why:            approvals.HeadlineSaysWhy,
			Kind:           approvalsubject.KindLegalReviewRequest,
			ProposedChange: body,
			DiffHash:       hex.EncodeToString(digest[:]),
			TargetType:     targetType,
			Summary:        fmt.Sprintf(systemtext.Copy(summaryCode, textlang.English), request.DealName),
			SystemText:     &systemtext.Message{Version: 1, Subject: systemtext.Sentence(summaryCode, systemtext.Text(request.DealName))},
			JoinPending:    true,
			Identity:       identity,
		})
		if err != nil && !errors.Is(err, approvals.ErrNoDecider) {
			return err
		}
	}
	return nil
}

// Settle withdraws every pending entry of the review and takes back the notice
// each one announced.
func (d Desk) Settle(ctx context.Context, tx pgx.Tx, reviewID ids.UUID) error {
	system := principal.SystemActing(ctx, actor)
	rows, err := tx.Query(ctx, `
		SELECT id FROM approval
		 WHERE kind = $1 AND status = 'pending' AND proposed_change->>'review_id' = $2`,
		approvalsubject.KindLegalReviewRequest, reviewID.String())
	if err != nil {
		return fmt.Errorf("legalreviewdesk: finding the review's entries: %w", err)
	}
	pending, err := pgx.CollectRows(rows, pgx.RowTo[ids.ApprovalID])
	if err != nil {
		return fmt.Errorf("legalreviewdesk: reading the review's entries: %w", err)
	}
	for _, id := range pending {
		if _, err := d.Approvals.WithdrawInTx(system, tx, id, settledNote); err != nil {
			return err
		}
		if _, err := d.Notices.Retract(system, tx, notices.ApprovalNoticeKey(id.UUID)); err != nil {
			return err
		}
	}
	return nil
}
