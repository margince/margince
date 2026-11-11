// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"time"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

type QuietPopulation struct {
	EntityType string
	PipelineID ids.UUID
	Members    storekit.ListMemberFilter
}

type quietQueryScope struct{ eligibility, reminder string }

func (s *Store) LastTouchInPopulation(ctx context.Context, cutoff time.Time, limit int, population QuietPopulation) ([]LastTouchCandidate, error) {
	return s.lastTouchBefore(ctx, cutoff, limit, "", &population)
}

func scopedQuietQuery(ctx context.Context, population *QuietPopulation, args *[]any) (*quietQueryScope, error) {
	if population == nil {
		return nil, nil
	}
	var table string
	switch population.EntityType {
	case "deal":
		if population.PipelineID.IsZero() || population.Members != nil {
			return nil, apperrors.ErrInvalidArgument
		}
		table = "deal"
	case "contact":
		table = "contact"
	case "company":
		table = "company"
	default:
		return nil, apperrors.ErrInvalidArgument
	}
	if table != "deal" {
		if population.Members == nil || !population.PipelineID.IsZero() {
			return nil, apperrors.ErrInvalidArgument
		}
	}
	if err := auth.Require(ctx, table, principal.ActionRead); err != nil {
		return nil, err
	}
	arg := func(value any) int { *args = append(*args, value); return len(*args) }
	clause := storekit.SQLf("q.entity_type=$%d AND EXISTS(SELECT 1 FROM %s r WHERE r.id=q.entity_id AND r.archived_at IS NULL", arg(table), table)
	if table == "deal" {
		clause += storekit.SQLf(" AND r.status='open' AND r.pipeline_id=$%d", arg(population.PipelineID))
	} else {
		members, err := population.Members("r.id", arg)
		if err != nil {
			return nil, err
		}
		clause += " AND (" + members + ")"
	}
	visibility, err := auth.ScopeClauseFor(ctx, table, "r", arg)
	if err != nil {
		return nil, err
	}
	if visibility != "" {
		clause += " AND (" + visibility + ")"
	}
	return &quietQueryScope{eligibility: clause + ")", reminder: openReminderHoldsEntityFrom(
		storekit.SQLf("t.source_system=ANY($%d::text[])", arg(provenance.EngineReminderSources())))}, nil
}
