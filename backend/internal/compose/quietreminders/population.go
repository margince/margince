// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package quietreminders

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/automation"
	"github.com/margince/margince/backend/internal/modules/collections"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

type Reader struct {
	activityScanAdapter
	lists *collections.Store
	deals *deals.Store
}

func NewReader(store *activities.Store, lists *collections.Store, records *deals.Store) Reader {
	return Reader{activityScanAdapter: activityScanAdapter{store: store}, lists: lists, deals: records}
}

func (a Reader) population(ctx context.Context, in automation.QuietPopulation) (activities.QuietPopulation, error) {
	if !in.PipelineID.IsZero() && in.ListID.IsZero() {
		pipeline, err := a.deals.GetPipeline(ctx, ids.From[ids.PipelineKind](in.PipelineID))
		if err != nil {
			return activities.QuietPopulation{}, err
		}
		if pipeline.ArchivedAt != nil {
			return activities.QuietPopulation{}, apperrors.ErrNotFound
		}
		return activities.QuietPopulation{EntityType: "deal", PipelineID: in.PipelineID, Readers: in.Readers}, nil
	}
	if in.ListID.IsZero() || !in.PipelineID.IsZero() {
		return activities.QuietPopulation{}, apperrors.ErrInvalidArgument
	}
	list, err := a.lists.ListForRule(ctx, ids.From[ids.ListKind](in.ListID))
	if err != nil {
		return activities.QuietPopulation{}, err
	}
	if list.Archived || list.Invalid || !slices.Contains(automation.QuietCustomerTypes(), list.EntityType) {
		return activities.QuietPopulation{}, &automation.ParamError{Field: "params.customer_list_id", Reason: "choose a current contact or company list"}
	}
	members, err := a.lists.MemberFilter(ctx, in.ListID, list.EntityType)
	if err != nil {
		return activities.QuietPopulation{}, err
	}
	return activities.QuietPopulation{EntityType: list.EntityType, Members: members, Readers: in.Readers}, nil
}

func (a Reader) ValidateQuietPopulation(ctx context.Context, in automation.QuietPopulation) error {
	_, err := a.population(ctx, in)
	return err
}

func (a Reader) QuietCandidates(ctx context.Context, in automation.QuietPopulation, cutoff time.Time, limit int) (automation.QuietCandidates, error) {
	population, err := a.population(ctx, in)
	if err != nil {
		return automation.QuietCandidates{}, err
	}
	filters := []storekit.ListMemberFilter{population.Members}
	for _, reader := range in.Readers {
		other, err := a.population(principal.WithActor(ctx, reader), in)
		if err != nil {
			return automation.QuietCandidates{}, err
		}
		filters = append(filters, other.Members)
	}
	if population.Members != nil && len(filters) > 1 {
		population.Members = func(idColumn string, arg func(any) int) (string, error) {
			var clauses []string
			for _, filter := range filters {
				clause, err := filter(idColumn, arg)
				if err != nil {
					return "", err
				}
				clauses = append(clauses, "("+clause+")")
			}
			return strings.Join(clauses, " AND "), nil
		}
	}
	rows, err := a.store.LastTouchInPopulation(ctx, cutoff, limit, population)
	if err != nil {
		return automation.QuietCandidates{}, err
	}
	out := automation.QuietCandidates{}
	for _, row := range rows {
		out.Total = row.Total
		out.Records = append(out.Records, automation.EntityAnchor{Ref: datasource.EntityRef{Type: datasource.EntityType(row.EntityType), ID: row.EntityID}, Anchor: row.LastTouch})
	}
	return out, nil
}
