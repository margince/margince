// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"net/http"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Handlers is the feed's HTTP surface: one read, no verbs.
//
// Every verb a card offers belongs to the record that owns it — approve goes
// to the approvals engine, complete to the activity, merge to the dedupe
// queue. A second door onto a decision would be a second place for its rules
// to live.
type Handlers struct {
	svc *Service
}

// NewHandlers binds the route to the assembler.
func NewHandlers(svc *Service) Handlers { return Handlers{svc: svc} }

// GetAttention answers the day: what needs deciding, what is planned, and what
// already ran.
func (h Handlers) GetAttention(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.Assemble(r.Context())
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

// GetTeamBoard answers what each teammate is carrying.
//
// No parameters: whose team it is comes from the principal, so a reader cannot
// ask about somebody else's. The row-scope tier that admits the read is checked
// in the service, beside the roster it draws.
func (h Handlers) GetTeamBoard(w http.ResponseWriter, r *http.Request) {
	out, err := h.svc.TeamBoard(r.Context())
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

// GetWorklist answers the same day as one ranked queue.
//
// It reads through the same assembler GetAttention does, so a lane added there
// reaches this queue by being classified rather than by being read a second
// time. Two readers of one day would drift the first time a lane changed.
func (h Handlers) GetWorklist(w http.ResponseWriter, r *http.Request, params crmcontracts.GetWorklistParams) {
	filter := ""
	if params.Filter != nil {
		// A filter the contract does not name is refused rather than answered
		// with an empty queue: a client sending `filter=deals` (for
		// `deals_at_risk`) would otherwise be told, in a 200, that the reader
		// has no work — which is the one answer this surface must never give
		// wrongly.
		if !params.Filter.Valid() {
			httperr.Write(w, r, httperr.Validation("filter", "unknown",
				"that is not a kind of work this queue can narrow to"))
			return
		}
		filter = string(*params.Filter)
	}
	limit := 0
	if params.Limit != nil {
		limit = *params.Limit
	}
	scope := ""
	if params.Scope != nil {
		if !params.Scope.Valid() {
			httperr.Write(w, r, httperr.Validation("scope", "unknown",
				"that is not a scope this queue can answer for"))
			return
		}
		scope = string(*params.Scope)
	}
	// The generated type already refused anything that is not a uuid, so an
	// unparseable owner never reaches the resolver.
	var owner ids.UUID
	if params.Owner != nil {
		// A named owner and a wider scope are two answers to one question, and
		// nothing defines which wins. Letting the owner take it silently left
		// the response echoing the scope that had been ignored — one rep's work
		// labelled `"scope": "unassigned"`, which is the most misleading
		// direction available. Refused, so the caller says which they meant.
		if scope != "" && scope != scopeMine {
			httperr.Write(w, r, httperr.Validation("owner", "conflicts_with_scope",
				"asking for one person's queue and for a wider scope are different questions; send one"))
			return
		}
		owner = ids.UUID(*params.Owner)
	}
	out, err := h.svc.Worklist(r.Context(), scope, filter, owner, limit)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}
