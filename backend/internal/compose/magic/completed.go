// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package magic

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// CompletedRead is the common admission and ordering behind Home and change history.
type CompletedRead struct {
	Lines                []crmcontracts.MagicLine
	Floor, CountWithheld bool
	NotShown             []crmcontracts.MagicNotShown
	Unavailable          []crmcontracts.WorklistSourceUnavailable
}

// CompletedChanges applies the same admission, grouping and paging to both surfaces.
func (s *Service) CompletedChanges(ctx context.Context, since, through time.Time, limit int) (CompletedRead, error) {
	if err := auth.RequireHumanOrItsAgent(ctx); err != nil {
		return CompletedRead{}, err
	}
	ctx = context.WithValue(ctx, completedThroughKey{}, through)
	receipt := crmcontracts.MagicReceipt{Done: []crmcontracts.MagicLine{}}
	width := limit + 1
	err := database.WithWorkspaceSnapshot(ctx, s.pool, func(ctx context.Context) error {
		return database.WithWorkspaceTx(ctx, s.pool, func(tx pgx.Tx) error { return s.drawDone(ctx, tx, since, width, &receipt) })
	})
	if err != nil {
		return CompletedRead{}, err
	}
	decided, floor, unavailable, err := s.decidedPage(ctx, receipt.Done, since, width)
	if err != nil {
		return CompletedRead{}, err
	}
	lines := mergeNewestFirst(receipt.Done, decided, width)
	for i := range lines {
		if lines[i].CountIsFloor != nil && *lines[i].CountIsFloor {
			floor = true
		}
	}
	out := CompletedRead{Lines: lines[:min(limit, len(lines))], Floor: floor || len(lines) > limit, NotShown: receipt.NotShown}
	if receipt.NotShownCountWithheld != nil {
		out.CountWithheld = *receipt.NotShownCountWithheld
	}
	out.Unavailable, err = withheldCompletedSources(ctx)
	if err != nil {
		return CompletedRead{}, err
	}
	if unavailable {
		out.Unavailable = append(out.Unavailable, crmcontracts.WorklistSourceUnavailable{Source: sourceStagedApproval, Reason: crmcontracts.WorklistSourceUnavailableReasonWithheld})
	}
	return out, nil
}

func withheldCompletedSources(ctx context.Context) ([]crmcontracts.WorklistSourceUnavailable, error) {
	types := slices.Sorted(maps.Keys(scopedTypes))
	types = append(types, typeActivity)
	sources := []crmcontracts.WorklistSourceUnavailable{}
	for _, kind := range types {
		err := auth.Require(ctx, kind, principal.ActionRead)
		if errors.Is(err, apperrors.ErrPermissionDenied) {
			sources = append(sources, crmcontracts.WorklistSourceUnavailable{Source: kind, Reason: crmcontracts.WorklistSourceUnavailableReasonWithheld})
			continue
		}
		if err != nil {
			return nil, err
		}
	}
	return sources, nil
}

type completedThroughKey struct{}

func completedThrough(ctx context.Context) *time.Time {
	through, ok := ctx.Value(completedThroughKey{}).(time.Time)
	if !ok || through.IsZero() {
		return nil
	}
	return &through
}

func completedCutoff(ctx context.Context, arg func(any) int) string {
	if through := completedThrough(ctx); through != nil {
		return fmt.Sprintf(" AND a.occurred_at <= $%d -- occurred_at raw: audit grouping keyset cutoff\n", arg(*through))
	}
	return ""
}
