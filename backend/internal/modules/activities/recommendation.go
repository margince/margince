// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

// TaskRecommendation resolves a suggestion inside the transaction that accepts it.
type TaskRecommendation func(context.Context, pgx.Tx, crmcontracts.RecommendationReceipt) (LogActivityInput, error)

// WithTaskRecommendation binds the current rule used by suggested task writes.
func (s *Store) WithTaskRecommendation(read TaskRecommendation) *Store {
	s.taskRecommendation = read
	return s
}

// WithTaskRecommendation binds the shared store guard on the web surface.
func (h Handlers) WithTaskRecommendation(read TaskRecommendation) Handlers {
	h.store.WithTaskRecommendation(read)
	return h
}

// WithTaskRecommendation binds the shared store guard on the tool surface.
func (p *Provider) WithTaskRecommendation(read TaskRecommendation) *Provider {
	p.store.WithTaskRecommendation(read)
	return p
}

func (s *Store) checkTaskRecommendation(ctx context.Context, tx pgx.Tx, in *LogActivityInput) error {
	if in.Recommendation == nil {
		return nil
	}
	if s.taskRecommendation == nil {
		return fmt.Errorf("this writer cannot accept a recommendation; use the task endpoint: %w", apperrors.ErrInvalidArgument)
	}
	current, err := s.taskRecommendation(ctx, tx, *in.Recommendation)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(taskIntent(*in), taskIntent(current)) {
		return fmt.Errorf("the suggested step changed; refresh it before accepting: %w", apperrors.ErrVersionSkew)
	}
	if in.RequestActivityID == nil {
		sourceID, err := acceptedRecommendationID(ctx, *in.Recommendation)
		if err != nil {
			return err
		}
		sourceSystem := provenance.AcceptedRecommendationSource
		in.SourceSystem, in.SourceID = &sourceSystem, &sourceID
	}

	return nil
}

// A committed acceptance is a read; current admission guards only a new write.
func (s *Store) replayedTaskRecommendation(ctx context.Context, tx pgx.Tx, in LogActivityInput) (crmcontracts.Activity, bool, error) {
	if in.Recommendation == nil || s.taskRecommendation == nil {
		return crmcontracts.Activity{}, false, nil
	}
	if in.Kind != string(crmcontracts.ActivityKindTask) {
		return crmcontracts.Activity{}, false, apperrors.ErrInvalidArgument
	}
	key, err := acceptedRecommendationID(ctx, *in.Recommendation)
	if err != nil {
		return crmcontracts.Activity{}, false, err
	}
	source := provenance.AcceptedRecommendationSource
	replay, err := replayedSourceActivity(ctx, tx, source, key)
	if replay == nil {
		return crmcontracts.Activity{}, false, err
	}
	return *replay, true, err
}

func taskIntent(in LogActivityInput) LogActivityInput {
	in.Recommendation = nil
	in.customColumns, in.customChecked = nil, false
	in.Origin = ""
	return in
}

func acceptedRecommendationID(ctx context.Context, receipt crmcontracts.RecommendationReceipt) (string, error) {
	actor, ok := principal.Actor(ctx)
	if !ok || actor.UserID.IsZero() {
		return "", apperrors.ErrPermissionDenied
	}
	receipt.CheckedAt = receipt.CheckedAt.UTC()
	encoded, err := json.Marshal(struct {
		User    string
		Receipt crmcontracts.RecommendationReceipt
	}{actor.UserID.String(), receipt})
	if err != nil {
		return "", fmt.Errorf("encode the accepted recommendation: %w", err)
	}
	sum := sha256.Sum256(encoded)
	return "v1:" + hex.EncodeToString(sum[:]), nil
}
