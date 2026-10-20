// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"github.com/jackc/pgx/v5"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/provider"
)

type MergeProviderRun struct {
	Before             provider.Run
	AfterState         provider.RunState
	AfterSkipReason    provider.SkipReason
	Moves              bool
	FingerprintChanges bool
}

type ProviderRunReader func(context.Context, pgx.Tx, string, string) (provider.Run, error)

func (s *Store) WithProviderRunReader(read ProviderRunReader) *Store {
	s.providerRunReader = read
	return s
}
func (p *Provider) WithProviderRunReader(read ProviderRunReader) *Provider {
	p.store.WithProviderRunReader(read)
	return p
}
func (h Handlers) WithProviderRunReader(read ProviderRunReader) Handlers {
	h.store.WithProviderRunReader(read)
	return h
}

func (s *Store) previewMergeProviderRuns(ctx context.Context, tx pgx.Tx, source, target ids.UUID) ([]MergeProviderRun, error) {
	affectedRuns, err := readMergeProviderEffects(ctx, tx, source, target, source)
	if err != nil {
		return nil, err
	}
	survivorRuns, err := readMergeProviderEffects(ctx, tx, target, source, source)
	if err != nil {
		return nil, err
	}
	affectedRuns = append(affectedRuns, survivorRuns...)
	if len(affectedRuns) > 500 {
		return nil, apperrors.ErrInvalidArgument
	}
	if len(affectedRuns) > 0 && s.providerRunReader == nil {
		return nil, apperrors.ErrInvalidArgument
	}
	out := make([]MergeProviderRun, 0, len(affectedRuns))
	for _, affected := range affectedRuns {
		run, err := s.providerRunReader(ctx, tx, affected.Contact.String(), affected.ID.String())
		if err != nil {
			return nil, err
		}
		effect := MergeProviderRun{Before: run, AfterState: run.State, AfterSkipReason: run.SkipReason, Moves: affected.Contact == source, FingerprintChanges: affected.Contact == source && affected.PaidState && affected.PaidTwin}
		if run.State == provider.RunQueued {
			if effect.Moves || affected.PaidTwin {
				effect.AfterState = provider.RunCancelled
			} else {
				effect.AfterState = provider.RunSkipped
				effect.AfterSkipReason = provider.SkipCategoryInFlight
			}
			effect.FingerprintChanges = true
		}
		out = append(out, effect)
	}
	return out, nil
}

type mergeProviderEffect struct {
	ID, Contact         ids.UUID
	PaidTwin, PaidState bool
}

func readMergeProviderEffects(ctx context.Context, tx pgx.Tx, subject, other, source ids.UUID) ([]mergeProviderEffect, error) {
	rows, err := tx.Query(ctx, `SELECT s.id, s.contact_id,
 `+mergePaidProviderTwin+` AS paid_twin,
 s.state IN `+mergePaidProviderStates+` AS paid_state
 FROM provider_run s WHERE s.contact_id = @subject
 AND (@moves OR (s.state = 'queued' AND (`+mergePaidProviderTwin+`
 OR EXISTS (SELECT 1 FROM provider_run q WHERE q.id = s.id AND `+mergeProviderCategoryTwin+`))))
 ORDER BY s.id LIMIT 501`, pgx.StrictNamedArgs{"source": source, "subject": subject, "other": other, "moves": subject == source})
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[mergeProviderEffect])
}
