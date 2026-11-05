// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/feedback"
)

// FeedbackCarryReader keeps preview reads under the owning module’s authorization policy.
type FeedbackCarryReader interface {
	PreviewFeedbackCarryTx(context.Context, pgx.Tx, string, ids.UUID, ids.UUID) ([]feedback.CarriedDecision, error)
}

// WithFeedbackCarryReader uses the owning module’s authorized reader so preview and execution share its policy.
func (s *Store) WithFeedbackCarryReader(reader FeedbackCarryReader) *Store {
	s.feedbackCarryReader = reader
	return s
}

// WithFeedbackCarryReader uses the owning module’s authorized reader so preview and execution share its policy.
func (p *Provider) WithFeedbackCarryReader(reader FeedbackCarryReader) *Provider {
	p.store.WithFeedbackCarryReader(reader)
	return p
}

// WithFeedbackCarryReader uses the owning module’s authorized reader so preview and execution share its policy.
func (h Handlers) WithFeedbackCarryReader(reader FeedbackCarryReader) Handlers {
	h.store.WithFeedbackCarryReader(reader)
	return h
}

func (s *Store) previewFeedbackCarry(ctx context.Context, tx pgx.Tx, kind string, source, target ids.UUID) ([]feedback.CarriedDecision, error) {
	if s.feedbackCarryReader != nil {
		return s.feedbackCarryReader.PreviewFeedbackCarryTx(ctx, tx, kind, source, target)
	}
	var held bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM ai_feedback WHERE subject_type = @kind AND subject_id = @source)`, pgx.StrictNamedArgs{recordKindParam: kind, mergeSourceKey: source}).Scan(&held); err != nil {
		return nil, err
	}
	if held {
		return nil, apperrors.ErrInvalidArgument
	}
	return []feedback.CarriedDecision{}, nil
}
