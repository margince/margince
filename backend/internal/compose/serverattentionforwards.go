// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"net/http"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// The attention and magic surfaces are named fields of Server rather than
// embedded handler sets, so none of their methods is promoted; each one the
// contract asks for is forwarded here by hand.

// GetAttention forwards the day's read to the assembled surface. Explicit
// because the field is named rather than embedded, so no method is promoted.
func (s Server) GetAttention(w http.ResponseWriter, r *http.Request) {
	s.attentionHandlers.GetAttention(w, r)
}

// GetMagic forwards the machinery's receipt to its own surface.
func (s Server) GetMagic(w http.ResponseWriter, r *http.Request, params crmcontracts.GetMagicParams) {
	s.magicHandlers.GetMagic(w, r, params)
}

// GetWorklist forwards the ranked read to the same assembled surface.
func (s Server) GetWorklist(w http.ResponseWriter, r *http.Request, params crmcontracts.GetWorklistParams) {
	s.attentionHandlers.GetWorklist(w, r, params)
}

// GetResponseMetrics forwards the reading of how fast the workspace replies.
func (s Server) GetResponseMetrics(
	w http.ResponseWriter, r *http.Request, params crmcontracts.GetResponseMetricsParams,
) {
	s.attentionHandlers.GetResponseMetrics(w, r, params)
}

// GetHiddenBacklog forwards the guardrail over the queue's own hiding rules.
func (s Server) GetHiddenBacklog(w http.ResponseWriter, r *http.Request) {
	s.attentionHandlers.GetHiddenBacklog(w, r)
}

// GetHandledForYou forwards the reader's own receipt of what was done.
func (s Server) GetHandledForYou(w http.ResponseWriter, r *http.Request) {
	s.attentionHandlers.GetHandledForYou(w, r)
}

// GetTeamExceptions forwards the lead's read of what is going wrong.
func (s Server) GetTeamExceptions(w http.ResponseWriter, r *http.Request) {
	s.attentionHandlers.GetTeamExceptions(w, r)
}

// GetTeamBoard forwards the manager's read of the same work.
func (s Server) GetTeamBoard(w http.ResponseWriter, r *http.Request) {
	s.attentionHandlers.GetTeamBoard(w, r)
}
