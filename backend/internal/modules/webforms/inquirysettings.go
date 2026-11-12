// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package webforms

import (
	"context"
	"net/http"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// InquirySettings joins policy and form writes without either module owning the other's rows.
type InquirySettings interface {
	HandleExisting(context.Context, ids.UUID, crmcontracts.WebsiteInquiryExistingRequest) (crmcontracts.WebsiteInquiryHandlingResults, error)
	Preview(context.Context, ids.UUID, crmcontracts.WebsiteInquiryPreviewRequest, time.Time) (crmcontracts.WebsiteInquiryHandlingPreview, error)
	Save(context.Context, ids.UUID, int64, crmcontracts.WebsiteInquiryHandlingRequest) (crmcontracts.WebsiteInquiryHandling, error)
}

// WithInquirySettings is a copy of the store that saves and runs inquiry
// policies through settings.
func (s *Store) WithInquirySettings(settings InquirySettings) *Store {
	clone := *s
	clone.settings = settings
	return &clone
}

// SaveHandling stores the inquiry policy of form id. The policy must name the
// same form.
func (s *Store) SaveHandling(ctx context.Context, id ids.UUID, version int64, in crmcontracts.WebsiteInquiryHandlingRequest) (crmcontracts.WebsiteInquiryHandling, error) {
	if err := auth.Require(ctx, rbacObject, principal.ActionUpdate); err != nil {
		return crmcontracts.WebsiteInquiryHandling{}, err
	}
	if ids.UUID(in.Policy.FormId) != id {
		return crmcontracts.WebsiteInquiryHandling{}, badInput("policy.form_id", "must name this website form")
	}
	if s.settings == nil {
		return crmcontracts.WebsiteInquiryHandling{}, apperrors.ErrNotFound
	}
	return s.settings.Save(ctx, id, version, in)
}

// SaveWebFormHandling serves a form's inquiry policy save. It needs If-Match.
func (h Handlers) SaveWebFormHandling(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, _ crmcontracts.SaveWebFormHandlingParams) {
	version, ok := httperr.IfMatchVersion(w, r)
	if !ok {
		return
	}
	var request crmcontracts.WebsiteInquiryHandlingRequest
	if !httperr.Decode(w, r, &request) {
		return
	}
	out, err := h.store.SaveHandling(r.Context(), ids.UUID(id), *version, request)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

// PreviewHandling shows what an inquiry policy would do on form id, and writes
// nothing.
func (s *Store) PreviewHandling(ctx context.Context, id ids.UUID, in crmcontracts.WebsiteInquiryPreviewRequest, now time.Time) (crmcontracts.WebsiteInquiryHandlingPreview, error) {
	if err := auth.Require(ctx, rbacObject, principal.ActionRead); err != nil {
		return crmcontracts.WebsiteInquiryHandlingPreview{}, err
	}
	if ids.UUID(in.Policy.FormId) != id {
		return crmcontracts.WebsiteInquiryHandlingPreview{}, badInput("policy.form_id", "must name this website form")
	}
	if s.settings == nil {
		return crmcontracts.WebsiteInquiryHandlingPreview{}, apperrors.ErrNotFound
	}
	return s.settings.Preview(ctx, id, in, now)
}

// PreviewWebFormHandling serves a form's inquiry policy preview.
func (h Handlers) PreviewWebFormHandling(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	var request crmcontracts.WebsiteInquiryPreviewRequest
	if !httperr.Decode(w, r, &request) {
		return
	}
	out, err := h.store.PreviewHandling(r.Context(), ids.UUID(id), request, h.now().UTC())
	if err != nil {
		writeErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

// HandleExisting runs the saved policy of form id over submissions accepted
// earlier.
func (s *Store) HandleExisting(ctx context.Context, id ids.UUID, in crmcontracts.WebsiteInquiryExistingRequest) (crmcontracts.WebsiteInquiryHandlingResults, error) {
	if err := auth.Require(ctx, rbacObject, principal.ActionUpdate); err != nil {
		return crmcontracts.WebsiteInquiryHandlingResults{}, err
	}
	if s.settings == nil {
		return crmcontracts.WebsiteInquiryHandlingResults{}, apperrors.ErrNotFound
	}
	return s.settings.HandleExisting(ctx, id, in)
}

// HandleExistingWebFormInquiries serves the run over a form's earlier submissions.
func (h Handlers) HandleExistingWebFormInquiries(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	var request crmcontracts.WebsiteInquiryExistingRequest
	if !httperr.Decode(w, r, &request) {
		return
	}
	out, err := h.store.HandleExisting(r.Context(), ids.UUID(id), request)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}
