// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package readerrendering

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/margince/margince/backend/internal/compose/modelfailure"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// The problem codes this endpoint adds to modelfailure's.
const (
	codeTooLong = "too_long"
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
	if req.Part != nil && req.ExpectedSourceVersion == nil {
		httperr.Write(w, r, httperr.Validation("expected_source_version", "required", "selecting a generated component requires its source version"))
		return
	}
	var part *string
	if req.Part != nil {
		if !req.Part.Valid() {
			httperr.Write(w, r, httperr.Validation("part", "invalid_enum", "part must be subject or body"))
			return
		}
		selected := string(*req.Part)
		part = &selected
	}
	out, err := h.svc.RenderPart(r.Context(), ids.From[ids.ActivityKind](ids.UUID(id)), locale, part, req.ExpectedSourceVersion)
	switch {
	case errors.Is(err, ErrNoModel):
		httperr.Write(w, r, &httperr.DetailedError{
			Status: http.StatusConflict, Code: httperr.CodeAINotConfigured,
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

// SuggestLabelTranslation implements POST /label-translations.
func (h Handlers) SuggestLabelTranslation(w http.ResponseWriter, r *http.Request) {
	var req crmcontracts.LabelTranslationRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Text) == "" {
		httperr.Write(w, r, httperr.Validation("text", "required", "send the label to translate"))
		return
	}
	if utf8.RuneCountInString(req.Text) > MaxLabelRunes {
		httperr.Write(w, r, httperr.Validation("text", codeTooLong, fmt.Sprintf("a label is at most %d characters", MaxLabelRunes)))
		return
	}
	if !req.Language.Valid() {
		httperr.Write(w, r, httperr.Validation("language", "invalid_enum", "language must be one of en, de, vi"))
		return
	}
	out, err := h.svc.SuggestLabel(r.Context(), req.Text, textlang.Lang(req.Language))
	switch {
	case errors.Is(err, ErrNoModel):
		httperr.Write(w, r, &httperr.DetailedError{
			Status: http.StatusConflict, Code: httperr.CodeAINotConfigured,
			Detail: "suggesting a translation needs an AI model; an administrator can bind one under Settings → AI",
		})
	case err != nil:
		modelfailure.Write(w, r, err)
	default:
		httperr.WriteJSON(w, http.StatusOK, out)
	}
}
