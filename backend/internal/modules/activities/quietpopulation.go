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

// QuietPopulation narrows a quiet scan to one pipeline's open deals, or to the
// contacts or companies Members admits. Readers also bound what it reads.
type QuietPopulation struct {
	EntityType string
	PipelineID ids.UUID
	Members    storekit.ListMemberFilter
	Readers    []principal.Principal
}

type quietQueryScope struct{ eligibility, reminder string }

// LastTouchInPopulation is LastTouchBefore for one population, with the total
// that match.
func (s *Store) LastTouchInPopulation(ctx context.Context, cutoff time.Time, limit int, population QuietPopulation) ([]LastTouchCandidate, error) {
	return s.scanLastTouch(ctx, cutoff, limit, "", &population)
}

// scopedQuietQuery narrows the quiet scan to one population, read under every
// reader the population names.
func scopedQuietQuery(ctx context.Context, population *QuietPopulation, args *[]any) (*quietQueryScope, error) {
	table, err := quietPopulationTable(population)
	if err != nil {
		return nil, err
	}
	if err := auth.RequireReadForReaders(ctx, table, population.Readers...); err != nil {
		return nil, err
	}
	arg := func(value any) int { *args = append(*args, value); return len(*args) }
	clause := storekit.SQLf("q.entity_type=$%d AND EXISTS(SELECT 1 FROM %s r WHERE r.id=q.entity_id AND r.archived_at IS NULL", arg(table), table)
	if table == linkEntityDeal {
		clause += storekit.SQLf(" AND r.status='open' AND r.pipeline_id=$%d", arg(population.PipelineID))
	} else {
		members, err := population.Members("r.id", arg)
		if err != nil {
			return nil, err
		}
		clause += " AND (" + members + ")"
	}
	visibility, err := auth.ScopeClauseForReaders(ctx, table, "r", arg, population.Readers...)
	if err != nil {
		return nil, err
	}
	if visibility != "" {
		clause += " AND (" + visibility + ")"
	}
	return &quietQueryScope{eligibility: clause + ")", reminder: openReminderHoldsEntityFrom(
		storekit.SQLf("t.source_system=ANY($%d::text[])", arg(provenance.EngineReminderSources())))}, nil
}

// quietPopulationTable names the table a population is drawn from. A deal
// population is a pipeline; a contact or company population is list members.
func quietPopulationTable(population *QuietPopulation) (string, error) {
	switch population.EntityType {
	case linkEntityDeal:
		if population.PipelineID.IsZero() || population.Members != nil {
			return "", apperrors.ErrInvalidArgument
		}
		return linkEntityDeal, nil
	case linkEntityContact, linkEntityCompany:
		if population.Members == nil || !population.PipelineID.IsZero() {
			return "", apperrors.ErrInvalidArgument
		}
		return population.EntityType, nil
	}
	return "", apperrors.ErrInvalidArgument
}
