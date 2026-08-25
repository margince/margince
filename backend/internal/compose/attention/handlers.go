// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"net/http"

	"github.com/gradionhq/margince/backend/internal/platform/httperr"
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
