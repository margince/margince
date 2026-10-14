// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// A deal owner asks legal to review a deal. The pack legal reads is built
// from the deal itself, so nobody briefs the reviewer by hand.
//
// The request is a write about the DEAL: it is gated on deal.update and
// audited on the deal. The decision is a write about the REVIEW, gated on the
// legal_review grant an admin hands to a reviewer (legalreviewdecide.go).

import (
	"context"
	"errors"
	"fmt"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

const legalReviewEntity = "legal_review"

// reviewIDKey names the review in the deal's audit images of a request and a
// withdrawal.
const reviewIDKey = "review_id"

// LegalReviewStatus is the Go spelling of the legal_review.status CHECK.
type LegalReviewStatus string

// The review lifecycle, mirroring the legal_review.status CHECK.
const (
	ReviewRequested     LegalReviewStatus = "requested"
	ReviewApproved      LegalReviewStatus = "approved"
	ReviewChangesNeeded LegalReviewStatus = "changes_needed"
	ReviewWithdrawn     LegalReviewStatus = "withdrawn"
)

// The bounds the table's CHECKs hold, counted in characters as the contract's
// maxLength is.
const (
	maxReviewNote     = 4000
	maxReviewComments = 8000
)

// PacketDocument is one contract, legal or offer document on the deal, as the
// requester could see it when they asked.
type PacketDocument struct {
	AttachmentID ids.UUID
	Category     string
	DocState     string
	ContentType  *string
	ByteSize     *int64
	AddedAt      time.Time
	// FromMessage marks a file that came with a message. Its audience is the
	// message's, so a reader outside it is not told the file exists.
	FromMessage bool
}

// DocumentName is how a pack document reads to a reader who may open it.
type DocumentName struct {
	Title    *string
	Filename string
}

// LegalReviewDocuments reads the files a review pack is about. The files
// belong to activities, so compose injects the reader. Both calls run under
// the caller's own principal, each on its own connection.
type LegalReviewDocuments interface {
	// ReviewDocuments lists the contract, legal and offer documents the
	// requester can see on the deal.
	ReviewDocuments(ctx context.Context, dealID ids.UUID) ([]PacketDocument, error)
	// OpenableDocuments answers which of these files the caller can open now,
	// with their names. A file the caller may not see is absent.
	OpenableDocuments(ctx context.Context, attachmentIDs []ids.UUID) (map[ids.UUID]DocumentName, error)
}

// WithLegalReviewDocuments wires the document reader the request needs.
func (h Handlers) WithLegalReviewDocuments(docs LegalReviewDocuments) Handlers {
	wired := *h.store
	wired.reviewDocs = docs
	h.store = &wired
	return h
}

// RequestLegalReviewInput is what a requester says beside the deal.
type RequestLegalReviewInput struct {
	Note             *string
	PreviousReviewID *ids.UUID
}

// LegalReviewOffError answers every legal review route while an admin has
// switched the feature off. 404, because the feature is absent rather than
// refused, and only after authorization, so it tells an outsider nothing.
type LegalReviewOffError struct{}

func (e *LegalReviewOffError) Error() string { return "deals: legal review is switched off" }

// MessageFault says what an admin can do about it.
func (e *LegalReviewOffError) MessageFault() (code, message string) {
	return "legal_review_off", "Legal review is switched off for this installation. An admin can switch it on in Settings."
}

func (e *LegalReviewOffError) Unwrap() error { return apperrors.ErrNotFound }

// LegalReviewOpenError is a second request on a deal that already has one
// waiting. It answers 409.
type LegalReviewOpenError struct{}

func (e *LegalReviewOpenError) Error() string {
	return "deals: this deal already has an open legal review"
}

// MessageFault names the way forward.
func (e *LegalReviewOpenError) MessageFault() (code, message string) {
	return "legal_review_already_open", "This deal already waits for a legal review. Withdraw it first to ask again."
}

func (e *LegalReviewOpenError) Unwrap() error { return apperrors.ErrConflict }

// LegalReviewDecidedError is a decision or withdrawal on a review that is no
// longer open. It answers 409.
type LegalReviewDecidedError struct{ Status LegalReviewStatus }

func (e *LegalReviewDecidedError) Error() string {
	return "deals: the legal review is already " + string(e.Status)
}

// MessageFault names the state the review is in, so the caller can reload.
func (e *LegalReviewDecidedError) MessageFault() (code, message string) {
	return "legal_review_decided", "This review is already " + string(e.Status) + ". Reload to see where it stands."
}

func (e *LegalReviewDecidedError) Unwrap() error { return apperrors.ErrConflict }

// requireReviewSeat answers the seated contact behind this call. Legal review
// is a human's request and a human's judgement. So a connector, the system
// principal and an agent are refused, though auth.RequireHuman admits two.
func requireReviewSeat(ctx context.Context) (ids.UUID, error) {
	p, ok := principal.Actor(ctx)
	if !ok || p.Type != principal.PrincipalHuman || p.UserID.IsZero() {
		return ids.UUID{}, fmt.Errorf("legal review is a seated contact's act: %w", apperrors.ErrPermissionDenied)
	}
	return p.UserID, nil
}

// ensureLegalReviewOn refuses while the installation has switched legal
// review off, read inside the caller's transaction.
func ensureLegalReviewOn(ctx context.Context, tx pgx.Tx) error {
	on, err := legalReviewOnForDecision(ctx, tx)
	if err != nil {
		return err
	}
	if !on {
		return &LegalReviewOffError{}
	}
	return nil
}

// boundedText refuses a text over its contract bound, naming the field.
func boundedText(field string, text *string, limit int) error {
	if text == nil || utf8.RuneCountInString(*text) <= limit {
		return nil
	}
	return &values.ParseError{
		Field: field, Code: "too_long",
		Message: fmt.Sprintf("%s may be at most %d characters", field, limit),
	}
}

// RequestLegalReview builds the pack and opens the request, in one
// transaction with its audit row and its event.
func (s *Store) RequestLegalReview(
	ctx context.Context, dealID ids.DealID, in RequestLegalReviewInput,
) (crmcontracts.LegalReviewSummary, error) {
	var out crmcontracts.LegalReviewSummary
	requester, err := requireRequester(ctx, in)
	if err != nil {
		return out, err
	}
	if s.reviewDocs == nil {
		return out, errors.New("deals: legal review needs a document reader, and none is wired")
	}
	// Before the transaction: the reader opens its own connection, and one
	// opened inside this transaction could deadlock against a lock it holds.
	docs, err := s.reviewDocs.ReviewDocuments(ctx, dealID.UUID)
	if err != nil {
		return out, err
	}
	err = s.Tx(ctx, func(tx pgx.Tx) error {
		if err := ensureLegalReviewOn(ctx, tx); err != nil {
			return err
		}
		if err := auth.EnsureWritableLive(ctx, tx, dealTable, dealID.UUID); err != nil {
			return err
		}
		// The pack and the fingerprint are read under it, and an advance
		// deciding on an earlier approval waits for this request.
		if _, err := storekit.LockRow(ctx, tx, dealTable, dealID.UUID, storekit.LiveOnly); err != nil {
			return err
		}
		if err := checkPreviousReview(ctx, tx, dealID, in.PreviousReviewID); err != nil {
			return err
		}
		packet, err := s.buildReviewPacket(ctx, tx, dealID, docs)
		if err != nil {
			return err
		}
		id, err := insertLegalReview(ctx, tx, dealID, requester, in, packet)
		if err != nil {
			return err
		}
		if err := refuteEarlierApprovals(ctx, tx, dealID, requester); err != nil {
			return err
		}
		hasNote := nonBlank(in.Note) != nil
		auditID, err := storekit.Audit(ctx, tx, "request_legal_review", "deal", dealID.UUID, nil,
			map[string]any{reviewIDKey: id.String(), "has_note": hasNote})
		if err != nil {
			return fmt.Errorf("audit the legal review request: %w", err)
		}
		if err := storekit.EmitPipelinePayload(ctx, tx, auditID,
			crmcontracts.InternalEventLegalReviewRequested{HasNote: hasNote}); err != nil {
			return fmt.Errorf("emit legal_review.requested: %w", err)
		}
		out, err = readReviewSummary(ctx, tx, id)
		return err
	})
	return out, err
}

// requireRequester answers the seat asking, after the gates a request takes.
// A request reads and writes the deal, and the pack reads its offer.
func requireRequester(ctx context.Context, in RequestLegalReviewInput) (ids.UUID, error) {
	requester, err := requireReviewSeat(ctx)
	if err != nil {
		return requester, err
	}
	if err := auth.Require(ctx, "deal", principal.ActionRead); err != nil {
		return requester, err
	}
	if err := auth.Require(ctx, "offer", principal.ActionRead); err != nil {
		return requester, err
	}
	if err := auth.Require(ctx, "deal", principal.ActionUpdate); err != nil {
		return requester, err
	}
	return requester, boundedText("note", in.Note, maxReviewNote)
}

// checkPreviousReview admits a repeated request only after a changes_needed
// answer on the same deal. The reviewer reads the history by this link, so it
// must point at a review that asked for changes.
func checkPreviousReview(ctx context.Context, tx pgx.Tx, dealID ids.DealID, previous *ids.UUID) error {
	if previous == nil {
		return nil
	}
	var status string
	err := tx.QueryRow(ctx,
		`SELECT status FROM legal_review WHERE id = $1 AND deal_id = $2`,
		*previous, dealID).Scan(&status)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("read the earlier legal review: %w", err)
	}
	if errors.Is(err, pgx.ErrNoRows) || LegalReviewStatus(status) != ReviewChangesNeeded {
		return &values.ParseError{
			Field: "previous_review_id", Code: "invalid_previous_review",
			Message: "previous_review_id names a review of this deal that asked for changes",
		}
	}
	return nil
}

// insertLegalReview writes the row, answering 409 on a deal that already has
// an open request: the partial unique index is what holds that.
func insertLegalReview(
	ctx context.Context, tx pgx.Tx, dealID ids.DealID, requester ids.UUID,
	in RequestLegalReviewInput, packet reviewPacket,
) (ids.UUID, error) {
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return ids.UUID{}, err
	}
	raw, err := packet.marshal()
	if err != nil {
		return ids.UUID{}, err
	}
	args := []any{
		dealID, requester, nonBlank(in.Note), in.PreviousReviewID, raw,
		packet.ValueMinor, packet.ValueCurrency, packet.Deal.ExpectedCloseDate, by,
	}
	var id ids.UUID
	err = tx.QueryRow(ctx, storekit.SQLf(`
		INSERT INTO legal_review (deal_id, requested_by, note, previous_review_id, packet,
		       value_minor, value_currency, expected_close_date, captured_by)
		VALUES (%s)
		RETURNING id`, storekit.Placeholders(args)), args...).Scan(&id)
	if storekit.IsUniqueViolation(err) {
		return id, &LegalReviewOpenError{}
	}
	if err != nil {
		return id, fmt.Errorf("insert legal review: %w", err)
	}
	return id, nil
}

// nonBlank keeps an empty note out of the row, so "no note" has one spelling.
func nonBlank(text *string) *string {
	if text == nil || !values.HasVisibleText(*text) {
		return nil
	}
	return text
}

// WithdrawLegalReview takes back an open request. It is the deal side's act,
// so it takes the deal's write gates rather than the reviewer's grant.
func (s *Store) WithdrawLegalReview(ctx context.Context, id ids.UUID) (crmcontracts.LegalReviewSummary, error) {
	var out crmcontracts.LegalReviewSummary
	if _, err := requireReviewSeat(ctx); err != nil {
		return out, err
	}
	if err := auth.Require(ctx, "deal", principal.ActionUpdate); err != nil {
		return out, err
	}
	err := s.Tx(ctx, func(tx pgx.Tx) error {
		if err := ensureLegalReviewOn(ctx, tx); err != nil {
			return err
		}
		current, lock, err := lockReview(ctx, tx, id)
		if err != nil {
			return err
		}
		dealID := ids.From[ids.DealKind](current.dealID)
		if err := auth.EnsureWritableLive(ctx, tx, dealTable, dealID.UUID); err != nil {
			return err
		}
		if current.status != ReviewRequested {
			return &LegalReviewDecidedError{Status: current.status}
		}
		patch := storekit.NewPatch()
		patch.Set("status", string(current.status), string(ReviewWithdrawn))
		if err := patch.ApplyLocked(ctx, tx, lock); err != nil {
			return fmt.Errorf("withdraw legal review: %w", err)
		}
		auditID, err := storekit.Audit(ctx, tx, "withdraw_legal_review", "deal", dealID.UUID,
			map[string]any{reviewIDKey: id.String(), filterStatus: string(current.status)},
			map[string]any{reviewIDKey: id.String(), filterStatus: string(ReviewWithdrawn)})
		if err != nil {
			return fmt.Errorf("audit the legal review withdrawal: %w", err)
		}
		if err := storekit.EmitPipelinePayload(ctx, tx, auditID,
			crmcontracts.InternalEventLegalReviewWithdrawn{}); err != nil {
			return fmt.Errorf("emit legal_review.withdrawn: %w", err)
		}
		out, err = readReviewSummary(ctx, tx, id)
		return err
	})
	return out, err
}

// reviewHead is the part of a review row its writers decide on.
type reviewHead struct {
	dealID      ids.UUID
	status      LegalReviewStatus
	requestedBy *ids.UUID
	dealOwner   *ids.UUID
}

// lockReview takes the review's row lock and reads what a decision rests on.
// The deal's row comes first, the order an advance and a request take. So a
// decision and a move on one deal serialize rather than deadlock.
func lockReview(ctx context.Context, tx pgx.Tx, id ids.UUID) (reviewHead, storekit.RowLock, error) {
	var out reviewHead
	if _, err := tx.Exec(ctx, `
		SELECT 1 FROM deal WHERE id = (SELECT deal_id FROM legal_review WHERE id = $1) FOR UPDATE`, id); err != nil {
		return out, storekit.RowLock{}, fmt.Errorf("lock the legal review's deal: %w", err)
	}
	lock, err := storekit.LockRow(ctx, tx, legalReviewEntity, id, storekit.IncludeArchived)
	if err != nil {
		return out, lock, err
	}
	var status string
	if err := tx.QueryRow(ctx,
		`SELECT r.deal_id, r.status, r.requested_by, d.owner_id
		   FROM legal_review r JOIN deal d ON d.id = r.deal_id WHERE r.id = $1`, id).
		Scan(&out.dealID, &status, &out.requestedBy, &out.dealOwner); err != nil {
		return out, lock, fmt.Errorf("read the legal review: %w", err)
	}
	out.status = LegalReviewStatus(status)
	return out, lock, nil
}
