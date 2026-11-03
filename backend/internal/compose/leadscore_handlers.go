// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The HTTP transport for GET /leads/{id}/score: the factor decomposition
// behind a lead's number, with the activities behind each behavioral factor
// named.
//
// The edge sits here rather than in `contacts` for the reason the strength
// seam does. The decomposition is computed in `contacts`, which may not
// import `activities`, so the receipts cannot be named where the response is
// built. The store call gates itself and filters every entry's sources
// through the caller's scope, so this file is edge plus naming and re-gates
// nothing.

import (
	"context"
	"net/http"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// explainScoreHistoryLimit bounds one page of the series. The read filters
// every entry's sources through the caller's scope, so an unbounded page is
// unbounded work as well as an unbounded response.
const explainScoreHistoryLimit = 50

// ExplainLeadScore implements GET /leads/{id}/score.
func (h receiptHandlers) ExplainLeadScore(
	w http.ResponseWriter, r *http.Request, id crmcontracts.Id, params crmcontracts.ExplainLeadScoreParams,
) {
	out, err := h.contacts.ExplainLeadScore(
		r.Context(), ids.From[ids.LeadKind](ids.UUID(id)), explainInputFrom(params))
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	named, err := h.namedScore(r.Context(), out)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, named)
}

// explainInputFrom reads the query dials, with a limit the caller cannot widen.
//
// A limit outside the bounds falls back to the page size rather than failing
// the read. The dial is a preference, and a reader who asks for more pages than
// the series holds is asking for the series.
func explainInputFrom(params crmcontracts.ExplainLeadScoreParams) contacts.ExplainLeadScoreInput {
	in := contacts.ExplainLeadScoreInput{Limit: explainScoreHistoryLimit}
	if params.History != nil {
		in.History = *params.History
	}
	if params.Cursor != nil {
		in.Cursor = *params.Cursor
	}
	if params.Limit != nil && *params.Limit > 0 && *params.Limit <= explainScoreHistoryLimit {
		in.Limit = *params.Limit
	}
	return in
}

// named answers the decomposition with its receipts named, or fails.
//
// A failure to name them fails the response rather than serving the bare
// counts, matching the strength read. A page that silently degraded to
// unopenable numbers is the defect this field removes, wearing a 200.
func (h receiptHandlers) namedScore(
	ctx context.Context, wire crmcontracts.LeadScoreExplanation,
) (crmcontracts.LeadScoreExplanation, error) {
	// Pointers into the response, so naming writes back where the reader
	// looks. The entries are addressed in place for the same reason.
	factors := make([]*crmcontracts.LeadScoreFactor, 0)
	collect := func(entry *crmcontracts.LeadScoreEntry) {
		if entry == nil || entry.Factors == nil {
			return
		}
		rows := *entry.Factors
		for i := range rows {
			factors = append(factors, &rows[i])
		}
		entry.Factors = &rows
	}
	collect(wire.Current)
	if wire.History != nil {
		history := *wire.History
		for i := range history {
			collect(&history[i])
		}
		wire.History = &history
	}

	groups := make([][]ids.UUID, 0, len(factors))
	for _, factor := range factors {
		if factor.SourceActivityIds == nil {
			groups = append(groups, nil)
			continue
		}
		groups = append(groups, activityIDsOf(*factor.SourceActivityIds))
	}
	named, err := namedActivityGroupsFor(ctx, h.pool, groups)
	if err != nil {
		return crmcontracts.LeadScoreExplanation{}, err
	}
	for i, factor := range factors {
		// A factor with no sources is left absent rather than given an empty
		// list. A fit factor has no activities to name. That is a different
		// statement from a behavioral factor whose sources this reader cannot
		// open.
		if factor.SourceActivityIds == nil {
			continue
		}
		factor.SourceActivities = &named[i]
	}
	return wire, nil
}
