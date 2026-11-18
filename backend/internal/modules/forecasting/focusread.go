// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package forecasting

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// TeamFocus is one team's focus list as its latest daily freeze stored it.
type TeamFocus struct {
	Team         ids.UUID
	SnapshotID   ids.UUID
	TakenAt      time.Time
	BaseCurrency string
	// OnTarget is nil when the team had no goal, so nothing was ranked.
	OnTarget *bool
	// Deals are the ranked rows the reader may see, by rank.
	Deals []Contribution
}

// FocusRead is the focus lists one reader may see. Withheld says the
// individual-performance switch left a team out.
type FocusRead struct {
	Teams    []TeamFocus
	Withheld bool
}

// FocusLists reads the latest daily focus list of each team in period, or of
// the team only names. Every list passes the snapshot lens. A team the reader
// may not assert about, or may not see under the individual-performance
// switch, is refused when named. Otherwise it is left out, and withheld says
// whether the switch left one out. The deals keep only what the reader may see.
func (s *Store) FocusLists(ctx context.Context, tx pgx.Tx, period Period, only *Scope) (FocusRead, error) {
	if err := auth.Require(ctx, "forecast", principal.ActionRead); err != nil {
		return FocusRead{}, err
	}
	if s.lens == nil {
		return FocusRead{}, errors.New("forecasting: no snapshot lens is bound, so a focus list cannot be narrowed")
	}
	// Named, the team must pass the lens. Unnamed, the reader's own population
	// must, which every reader may assert about; each team then passes on its own.
	scope, team := Scope{Kind: ScopeUnset}, (*ids.UUID)(nil)
	if only != nil {
		scope, team = *only, only.ID
	}
	if _, err := s.lens(ctx, tx, scope, nil, nil); err != nil {
		return FocusRead{}, err
	}
	var out FocusRead
	freezes, err := latestTeamFreezes(ctx, tx, period, team)
	if err != nil {
		return FocusRead{}, err
	}
	for _, list := range freezes {
		rows, err := tx.Query(ctx, ContributionSelect+`
			WHERE c.snapshot_id = $1 AND c.focus_rank IS NOT NULL
			ORDER BY c.focus_rank`, list.SnapshotID)
		if err != nil {
			return FocusRead{}, fmt.Errorf("forecasting: reading a focus list: %w", err)
		}
		ranked, err := pgx.CollectRows(rows, CollectContribution)
		if err != nil {
			return FocusRead{}, fmt.Errorf("forecasting: collecting a focus list: %w", err)
		}
		list.Deals, err = s.lens(ctx, tx, Scope{Kind: ScopeTeam, ID: &list.Team}, nil, ranked)
		switch {
		case team == nil && errors.As(err, new(auth.IndividualPerformanceOffError)):
			out.Withheld = true
			continue
		case team == nil && errors.Is(err, apperrors.ErrNotFound):
			continue
		case err != nil:
			return FocusRead{}, err
		}
		out.Teams = append(out.Teams, list)
	}
	return out, nil
}

// latestTeamFreezes is the newest daily freeze of each team over the whole
// pipeline in period, by team id.
func latestTeamFreezes(ctx context.Context, tx pgx.Tx, period Period, team *ids.UUID) ([]TeamFocus, error) {
	rows, err := tx.Query(ctx, `
		SELECT DISTINCT ON (scope_id) scope_id, id, taken_at, base_currency, focus_on_target
		FROM forecast_snapshot
		WHERE scope_kind = $1 AND trigger = $2 AND pipeline_id IS NULL
		  AND period_start = $3 AND period_end = $4
		  AND ($5::uuid IS NULL OR scope_id = $5)
		ORDER BY scope_id, taken_at DESC`,
		ScopeTeam, TriggerDaily, period.StartDate, period.EndDate, team)
	if err != nil {
		return nil, fmt.Errorf("forecasting: reading the latest team freezes: %w", err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (TeamFocus, error) {
		var out TeamFocus
		err := row.Scan(&out.Team, &out.SnapshotID, &out.TakenAt, &out.BaseCurrency, &out.OnTarget)
		return out, err
	})
}

// GetForecastFocus answers the focus lists of the current quarter.
func (h Handlers) GetForecastFocus(w http.ResponseWriter, r *http.Request, params crmcontracts.GetForecastFocusParams) {
	var out crmcontracts.ForecastFocus
	err := h.store.InTx(r.Context(), func(ctx context.Context, tx pgx.Tx) error {
		period, _, err := h.period(ctx, tx, PeriodQuarter, h.now())
		if err != nil {
			return err
		}
		var only *Scope
		if params.Team != nil {
			team, err := readScope(ScopeTeam, params.Team)
			if err != nil {
				return err
			}
			only = &team
		}
		focus, err := h.store.FocusLists(ctx, tx, period, only)
		if err != nil {
			return err
		}
		if focus, err = h.readerFocus(ctx, tx, focus); err != nil {
			return err
		}
		out, err = focusToWire(period, focus)
		return err
	})
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

// readerFocus keeps the listed deals the reader may see one by one, by the
// rule GET /forecast?contributions=true applies. With individual performance
// off, that is the reader's own deals, with no owner named.
func (h Handlers) readerFocus(ctx context.Context, tx pgx.Tx, focus FocusRead) (FocusRead, error) {
	if h.readerRows == nil {
		return FocusRead{}, errors.New("forecasting: no reader lens is bound, so a focus list cannot be narrowed")
	}
	var rows []Contribution
	for _, list := range focus.Teams {
		rows = append(rows, list.Deals...)
	}
	visible, _, err := h.readerRows(ctx, tx, rows)
	if err != nil {
		return FocusRead{}, err
	}
	// Each row's Focus is its own allocation, so it names one row of one list
	// even when a deal stands on two teams' lists.
	kept := make(map[*Focus]Contribution, len(visible))
	for _, row := range visible {
		kept[row.Focus] = row
	}
	for i := range focus.Teams {
		deals := make([]Contribution, 0, len(focus.Teams[i].Deals))
		for _, deal := range focus.Teams[i].Deals {
			if row, ok := kept[deal.Focus]; ok {
				deals = append(deals, row)
			}
		}
		focus.Teams[i].Deals = deals
	}
	return focus, nil
}

func focusToWire(period Period, focus FocusRead) (crmcontracts.ForecastFocus, error) {
	out := crmcontracts.ForecastFocus{
		PeriodStart: openapi_types.Date{Time: period.StartDate},
		PeriodEnd:   openapi_types.Date{Time: period.EndDate},
		Teams:       make([]crmcontracts.ForecastFocusTeam, 0, len(focus.Teams)),
		Withheld:    focus.Withheld,
	}
	for _, list := range focus.Teams {
		team := crmcontracts.ForecastFocusTeam{
			TeamId: openapi_types.UUID(list.Team), SnapshotId: openapi_types.UUID(list.SnapshotID),
			TakenAt: list.TakenAt, BaseCurrency: list.BaseCurrency,
			Status: focusStatus(list.OnTarget), Deals: make([]crmcontracts.ForecastFocusDeal, 0, len(list.Deals)),
		}
		for _, deal := range list.Deals {
			wire, err := focusDealToWire(deal)
			if err != nil {
				return out, err
			}
			team.Deals = append(team.Deals, wire)
		}
		out.Teams = append(out.Teams, team)
	}
	return out, nil
}

func focusStatus(onTarget *bool) crmcontracts.ForecastFocusTeamStatus {
	switch {
	case onTarget == nil:
		return crmcontracts.ForecastFocusTeamStatusNoGoal
	case *onTarget:
		return crmcontracts.ForecastFocusTeamStatusOnTarget
	default:
		return crmcontracts.ForecastFocusTeamStatusBehind
	}
}

// focusDealToWire renders one ranked row. The lens nulls a masked deal's base
// amount, and the weighted amount goes with it.
func focusDealToWire(c Contribution) (crmcontracts.ForecastFocusDeal, error) {
	dealID, err := ids.Parse(c.DealID)
	if err != nil {
		return crmcontracts.ForecastFocusDeal{}, fmt.Errorf("forecasting: a focus row names a deal id it cannot parse: %w", err)
	}
	if c.Focus == nil {
		return crmcontracts.ForecastFocusDeal{}, errors.New("forecasting: a focus row was read without its rank")
	}
	out := crmcontracts.ForecastFocusDeal{
		Rank: c.Focus.Rank, DealId: openapi_types.UUID(dealID), DealName: c.Name, OwnerId: optionalWireID(c.Owner),
		Reason: crmcontracts.ForecastFocusReason{
			ClosesInDays: c.Focus.Reason.ClosesInDays, CloseProvisional: c.Focus.Reason.CloseProvisional,
			Signals: wireSignals(c.Focus.Reason.Signals),
		},
	}
	if c.BaseMinor != nil {
		out.WeightedMinor = new(c.WeightedMinor)
	}
	if c.EffectiveClose != nil {
		out.EffectiveCloseDate = &openapi_types.Date{Time: *c.EffectiveClose}
	}
	return out, nil
}
