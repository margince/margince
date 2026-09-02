// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The weekly retrospective's routes, forwarded to the handlers that own them.
//
// Forwarded rather than embedded: briefs.Handlers already occupies the
// unqualified name on Server, so the weekly's two reads are named here.

import (
	"net/http"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// ListWeeklyReviews implements (GET /weekly-reviews).
func (s Server) ListWeeklyReviews(w http.ResponseWriter, r *http.Request) {
	s.weeklyHandlers.ListWeeklyReviews(w, r)
}

// GetTeamWeeklyReview implements (GET /weekly-reviews/team).
func (s Server) GetTeamWeeklyReview(
	w http.ResponseWriter, r *http.Request, params crmcontracts.GetTeamWeeklyReviewParams,
) {
	s.weeklyHandlers.GetTeamWeeklyReview(w, r, params)
}

// GetLatestWeeklyReview implements (GET /weekly-reviews/latest).
func (s Server) GetLatestWeeklyReview(
	w http.ResponseWriter, r *http.Request, params crmcontracts.GetLatestWeeklyReviewParams,
) {
	s.weeklyHandlers.GetLatestWeeklyReview(w, r, params)
}
