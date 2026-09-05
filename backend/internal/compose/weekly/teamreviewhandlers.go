// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package weekly

import (
	"net/http"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// GetTeamWeeklyReview answers a team's frozen week.
//
// It serves a snapshot that was WRITTEN when the week closed and never
// assembles one, for the reason the per-rep read gives: a read that could
// re-derive the week would answer differently depending on when it was asked.
func (h Handlers) GetTeamWeeklyReview(
	w http.ResponseWriter, r *http.Request, params crmcontracts.GetTeamWeeklyReviewParams,
) {
	var week *time.Time
	if params.Week != nil {
		day := params.Week.Time
		week = &day
	}
	review, err := h.engine.LatestTeamReview(r.Context(), ids.UUID(params.Team), week)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, teamReviewToWire(review))
}

// teamReviewToWire puts the snapshot on the contract's shape.
func teamReviewToWire(review TeamReview) crmcontracts.TeamWeeklyReview {
	reps := make([]crmcontracts.TeamWeeklyRep, 0, len(review.Reps))
	for _, rep := range review.Reps {
		reps = append(reps, crmcontracts.TeamWeeklyRep{
			UserId:          openapi_types.UUID(rep.UserID),
			DisplayName:     rep.DisplayName,
			DealsWon:        rep.DealsWon,
			LeadsBreached:   rep.LeadsBreached,
			MeetingsHeld:    rep.MeetingsHeld,
			CommitmentsDue:  rep.CommitmentsDue,
			CommitmentsKept: rep.CommitmentsKept,
			HelpRequested:   rep.HelpRequested,
			FocusKind:       crmcontracts.TeamWeeklyRepFocusKind(rep.FocusKind),
			FocusLabel:      rep.FocusLabel,
		})
	}
	c := review.Counts
	return crmcontracts.TeamWeeklyReview{
		Id:             openapi_types.UUID(review.ID),
		TeamId:         openapi_types.UUID(review.TeamID),
		TeamName:       review.TeamName,
		LocalWeekStart: openapi_types.Date{Time: review.LocalWeekStart},
		GeneratedAt:    review.GeneratedAt,
		AsOf:           review.AsOf,
		RepsUnread:     &review.RepsUnread,
		Counts: crmcontracts.TeamWeeklyCounts{
			RepsCounted: c.RepsCounted,
			DealsWon:    c.DealsWon, DealsLost: c.DealsLost, DealsMoved: c.DealsMoved,
			LeadsRouted: c.LeadsRouted, LeadsAnsweredInTarget: c.LeadsAnsweredInTarget,
			LeadsBreached: c.LeadsBreached,
			MeetingsHeld:  c.MeetingsHeld, MeetingsWithNextStep: c.MeetingsWithNextStep,
			CommitmentsDue: c.CommitmentsDue, CommitmentsKept: c.CommitmentsKept,
		},
		Pipeline: pipelineToWire(review.Money),
		Reps:     reps,
		// Derived here rather than stored: it is an order over the reps this
		// same call assembled, so it cannot go stale, and a snapshot written
		// before the agenda existed answers one anyway.
		Agenda: agendaToWire(agendaOrder(review.Reps)),
	}
}

// agendaToWire renders the agenda's order in the contract's uuid type.
func agendaToWire(order []ids.UUID) []openapi_types.UUID {
	agenda := make([]openapi_types.UUID, 0, len(order))
	for _, id := range order {
		agenda = append(agenda, openapi_types.UUID(id))
	}
	return agenda
}
