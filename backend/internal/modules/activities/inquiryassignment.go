// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"errors"
	"net/http"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// InquiryAssignment plans and writes the hand-on of a website inquiry task.
// Compose wires it, because the work spans leads and contacts.
type InquiryAssignment interface {
	Preview(context.Context, ids.ActivityID, crmcontracts.WebsiteInquiryAssignmentRequest) (crmcontracts.WebsiteInquiryAssignment, error)
	Apply(context.Context, ids.ActivityID, int64, crmcontracts.WebsiteInquiryAssignmentRequest) (crmcontracts.WebsiteInquiryAssignment, error)
}

// WithInquiryAssignment wires the inquiry hand-on into the store.
func (s *Store) WithInquiryAssignment(assignment InquiryAssignment) *Store {
	s.inquiryAssignment = assignment
	return s
}

// WithInquiryAssignment hands assignment to the handlers' store.
func (h Handlers) WithInquiryAssignment(assignment InquiryAssignment) Handlers {
	h.store.WithInquiryAssignment(assignment)
	return h
}

func (s *Store) checkInquiryAssignment(ctx context.Context, in crmcontracts.WebsiteInquiryAssignmentRequest) error {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return err
	}
	if err := auth.Require(ctx, "activity", principal.ActionUpdate); err != nil {
		return err
	}
	if err := auth.RequireHumanOrItsAgent(ctx); err != nil {
		return err
	}
	if (in.Mode != crmcontracts.WebsiteInquiryAssignmentRequestModeTake && in.Mode != crmcontracts.WebsiteInquiryAssignmentRequestModeAssignAndFollowUp) || ids.UUID(in.RecipientId).IsZero() {
		return apperrors.ErrInvalidArgument
	}
	if s.inquiryAssignment == nil {
		return errors.New("activities: inquiry assignment is not configured")
	}
	return nil
}

// PreviewInquiryAssignment plans a hand-on and writes nothing.
func (s *Store) PreviewInquiryAssignment(ctx context.Context, id ids.ActivityID, in crmcontracts.WebsiteInquiryAssignmentRequest) (crmcontracts.WebsiteInquiryAssignment, error) {
	if err := s.checkInquiryAssignment(ctx, in); err != nil {
		return crmcontracts.WebsiteInquiryAssignment{}, err
	}
	return s.inquiryAssignment.Preview(ctx, id, in)
}

// AssignInquiry writes a hand-on. The request must name the record and the
// versions the caller previewed.
func (s *Store) AssignInquiry(ctx context.Context, id ids.ActivityID, version int64, in crmcontracts.WebsiteInquiryAssignmentRequest) (crmcontracts.WebsiteInquiryAssignment, error) {
	if err := s.checkInquiryAssignment(ctx, in); err != nil {
		return crmcontracts.WebsiteInquiryAssignment{}, err
	}
	if version < 1 || in.RecordVersion == nil || in.RecordId == nil || in.RecordType == nil {
		return crmcontracts.WebsiteInquiryAssignment{}, apperrors.ErrInvalidArgument
	}
	if *in.RecordVersion < 1 || ids.UUID(*in.RecordId).IsZero() || (*in.RecordType != crmcontracts.WebsiteInquiryAssignmentRequestRecordTypeLead && *in.RecordType != crmcontracts.WebsiteInquiryAssignmentRequestRecordTypeContact) {
		return crmcontracts.WebsiteInquiryAssignment{}, apperrors.ErrInvalidArgument
	}
	return s.inquiryAssignment.Apply(ctx, id, version, in)
}

// PreviewWebsiteInquiryAssignment serves the assignment preview of an inquiry task.
func (h Handlers) PreviewWebsiteInquiryAssignment(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	var in crmcontracts.WebsiteInquiryAssignmentRequest
	if !httperr.Decode(w, r, &in) {
		return
	}
	out, err := h.store.PreviewInquiryAssignment(r.Context(), pathID[ids.ActivityKind](id), in)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

// AssignWebsiteInquiry serves an inquiry task's hand-on. It needs If-Match.
func (h Handlers) AssignWebsiteInquiry(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, _ crmcontracts.AssignWebsiteInquiryParams) {
	version, ok := httperr.IfMatchVersion(w, r)
	if !ok {
		return
	}
	if version == nil {
		writeStoreErr(w, r, apperrors.ErrInvalidArgument)
		return
	}
	var in crmcontracts.WebsiteInquiryAssignmentRequest
	if !httperr.Decode(w, r, &in) {
		return
	}
	out, err := h.store.AssignInquiry(r.Context(), pathID[ids.ActivityKind](id), *version, in)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}
