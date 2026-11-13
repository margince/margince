// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package rewrite

import (
	"errors"
	"net/http"
	"unicode/utf8"

	"github.com/margince/margince/backend/internal/compose/modelfailure"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/draftfloor"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// The problem codes this endpoint adds to modelfailure's.
const (
	codeAINotConfigured = "ai_not_configured"
	codeRestricted      = "subject_restricted"
)

// Handlers shadows the generated RewriteText stub.
type Handlers struct {
	svc *Service
}

// NewHandlers binds the transport to a ready service.
func NewHandlers(svc *Service) Handlers {
	return Handlers{svc: svc}
}

// RewriteText implements POST /rewrites.
func (h Handlers) RewriteText(w http.ResponseWriter, r *http.Request) {
	var req crmcontracts.RewriteRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	if fault := enumFault(req); fault != nil {
		httperr.Write(w, r, fault)
		return
	}
	ask := Ask{
		Envelope: Envelope(req.Envelope), Verb: Verb(req.Verb), Subject: deref(req.Subject), Body: deref(req.Body),
		Reader: textlang.Lang(req.Language),
		About:  ids.Ref{Type: string(req.About.EntityType), ID: ids.UUID(req.About.EntityId)},
	}
	out, err := h.svc.Rewrite(r.Context(), ask)
	switch {
	case errors.Is(err, ErrNoModel):
		httperr.Write(w, r, &httperr.DetailedError{
			Status: http.StatusConflict, Code: codeAINotConfigured,
			Detail: "rewriting words needs an AI model; an administrator can bind one under Settings → AI",
		})
	case errors.Is(err, model.ErrSubjectRestricted):
		httperr.Write(w, r, &httperr.DetailedError{
			Status: http.StatusConflict, Code: codeRestricted,
			Detail: "a contact on this record asked that their data not be processed, so no AI model reads these words",
		})
	case errors.Is(err, ErrVerbNotOffered):
		httperr.Write(w, r, httperr.Validation("verb", "invalid_enum", "a note offers polish only"))
	case errors.Is(err, ErrNothingToRewrite):
		httperr.Write(w, r, httperr.Validation("body", "required", "type some words to rewrite"))
	case errors.Is(err, ErrTooLong):
		httperr.Write(w, r, httperr.TooLong("body", MaxSourceRunes, utf8.RuneCountInString(ask.Subject)+utf8.RuneCountInString(ask.Body)))
	case err != nil:
		modelfailure.Write(w, r, err)
	default:
		answer := crmcontracts.Rewrite{
			Subject: out.Subject, Body: out.Body,
			Language:      crmcontracts.RewriteLanguage(out.Language),
			AiDisclosure:  draftfloor.AIProvenanceNotice(out.Language),
			VoiceDegraded: &out.Voice.Degraded,
		}
		if out.Voice.OK {
			answer.VoiceProfileVersion = new(out.Voice.Version.ProfileVersion)
		}
		httperr.WriteJSON(w, http.StatusOK, answer)
	}
}

// enumFault refuses a value outside the contract's lists, naming its field.
func enumFault(req crmcontracts.RewriteRequest) error {
	switch {
	case !req.Envelope.Valid():
		return httperr.Validation("envelope", "invalid_enum", "envelope must be email or note")
	case !req.Verb.Valid():
		return httperr.Validation("verb", "invalid_enum", "verb must be polish, shorter, warmer, formal or deadline")
	case !req.Language.Valid():
		return httperr.Validation("language", "invalid_enum", "language must be one of en, de, vi")
	case !req.About.EntityType.Valid():
		return httperr.Validation("about", "invalid_enum", "about names a contact, company, deal, lead, project or activity")
	}
	return nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
