// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package forecasting

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ManagerRequest is one open deal a seller asked the team's manager to look at.
type ManagerRequest struct {
	DealID      ids.UUID
	Name        string
	OwnerID     *ids.UUID
	RequestedBy *ids.UUID
	RequestedAt time.Time
	Reasons     []string
}

// ManagerRequestsFunc lists the requested deals the reader may read, narrowed
// to one team's members when team is given. Injected, because the request is a
// fact on the deal and the deals module owns it.
type ManagerRequestsFunc func(ctx context.Context, tx pgx.Tx, team *ids.UUID) ([]ManagerRequest, error)

// WithManagerRequests binds the reader of the requested deals a focus list
// shows beside its frozen ranking. Unbound, the list carries none.
func (h Handlers) WithManagerRequests(read ManagerRequestsFunc) Handlers {
	h.managerRequests = read
	return h
}

// requestedDeals is the section of requested deals, empty rather than null.
// With individual performance off a request would name a colleague's deal, so
// the section is left empty, as the ranked list shows only the reader's own.
func (h Handlers) requestedDeals(ctx context.Context, tx pgx.Tx, only *Scope) ([]crmcontracts.ForecastFocusRequested, error) {
	out := []crmcontracts.ForecastFocusRequested{}
	if allowed, bound := auth.IndividualPerformanceOf(ctx); h.managerRequests == nil || (bound && !allowed) {
		return out, nil
	}
	var team *ids.UUID
	if only != nil {
		team = only.ID
	}
	rows, err := h.managerRequests(ctx, tx, team)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		wire := crmcontracts.ForecastFocusRequested{
			DealId: openapi_types.UUID(row.DealID), DealName: row.Name, RequestedAt: row.RequestedAt,
			OwnerId: optionalUUID(row.OwnerID), RequestedBy: optionalUUID(row.RequestedBy),
			Reasons: make([]crmcontracts.ForecastFocusRequestedReasons, 0, len(row.Reasons)),
		}
		for _, reason := range row.Reasons {
			wire.Reasons = append(wire.Reasons, crmcontracts.ForecastFocusRequestedReasons(reason))
		}
		out = append(out, wire)
	}
	return out, nil
}

func optionalUUID(id *ids.UUID) *openapi_types.UUID {
	if id == nil {
		return nil
	}
	return new(openapi_types.UUID(*id))
}
