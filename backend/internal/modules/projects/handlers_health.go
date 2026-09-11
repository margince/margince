// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package projects

import (
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ListProjectHealthAssessments answers a project's judgements, newest first.
func (h Handlers) ListProjectHealthAssessments(
	w http.ResponseWriter, r *http.Request,
	id crmcontracts.Id, params crmcontracts.ListProjectHealthAssessmentsParams,
) {
	out, page, err := h.store.ListHealth(r.Context(), pathID[ids.ProjectKind](id), params.Limit)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, crmcontracts.ProjectHealthAssessmentListResponse{
		Data: out,
		Page: pageInfo(page),
	})
}

// CreateProjectHealthAssessment records how a project is going.
func (h Handlers) CreateProjectHealthAssessment(
	w http.ResponseWriter, r *http.Request,
	id crmcontracts.Id, _ crmcontracts.CreateProjectHealthAssessmentParams,
) {
	var req crmcontracts.CreateProjectHealthAssessmentRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	out, err := h.store.RecordHealth(r.Context(), RecordHealthInput{
		ProjectID:    pathID[ids.ProjectKind](id),
		State:        req.State,
		Note:         req.Note,
		AssessedAt:   req.AssessedAt,
		SourceAuthor: req.SourceAuthor,
	})
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusCreated, out)
}

// CorrectProjectHealthAssessment replaces one reading, keeping when it was
// said. The effective time is not in the request at all: it comes from the row
// being corrected, so a correction cannot quietly move a judgement to another
// day.
func (h Handlers) CorrectProjectHealthAssessment(
	w http.ResponseWriter, r *http.Request,
	id crmcontracts.Id, assessmentID openapi_types.UUID,
	_ crmcontracts.CorrectProjectHealthAssessmentParams,
) {
	var req crmcontracts.CreateProjectHealthCorrectionRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	out, err := h.store.CorrectHealth(r.Context(), CorrectHealthInput{
		ProjectID:    pathID[ids.ProjectKind](id),
		AssessmentID: ids.UUID(assessmentID),
		State:        req.State,
		Note:         req.Note,
		SourceAuthor: req.SourceAuthor,
	})
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusCreated, out)
}
