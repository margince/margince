// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package readerrendering

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/margince/margince/backend/internal/compose/modelfailure"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The problem codes this endpoint adds to modelfailure's.
const (
	codeAINotConfigured = "ai_not_configured"
	codeTooLong         = "too_long"
)

// Handlers shadows the generated reader-rendering stub.
type Handlers struct {
	svc *Service
}

// NewHandlers binds the transport to a ready service.
func NewHandlers(svc *Service) Handlers {
	return Handlers{svc: svc}
}

// RenderActivityForReader implements POST /activities/{id}/reader-rendering.
func (h Handlers) RenderActivityForReader(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	var req crmcontracts.ActivityReaderRenderingRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	if req.Locale != nil && !req.Locale.Valid() {
		httperr.Write(w, r, httperr.Validation("locale", "invalid_enum", "locale must be one of en, de, vi"))
		return
	}
	var locale *string
	if req.Locale != nil {
		chosen := string(*req.Locale)
		locale = &chosen
	}
	out, err := h.svc.Render(r.Context(), ids.From[ids.ActivityKind](ids.UUID(id)), locale)
	switch {
	case errors.Is(err, ErrNoModel):
		httperr.Write(w, r, &httperr.DetailedError{
			Status: http.StatusConflict, Code: codeAINotConfigured,
			Detail: "showing a message in your language needs an AI model; an administrator can bind one under Settings → AI",
		})
	case errors.Is(err, ErrTooLong):
		httperr.Write(w, r, &httperr.DetailedError{
			Status: http.StatusUnprocessableEntity, Code: codeTooLong,
			Detail: fmt.Sprintf("this text is longer than the %d characters one answer can carry; read the original", MaxSourceRunes),
		})
	case err != nil:
		modelfailure.Write(w, r, err)
	default:
		httperr.WriteJSON(w, http.StatusOK, out)
	}
}
