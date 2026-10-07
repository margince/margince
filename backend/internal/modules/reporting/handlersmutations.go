// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package reporting

import (
	"net/http"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// CreateReportingReport decodes a definition before the service records its first revision.
func (h Handlers) CreateReportingReport(w http.ResponseWriter, r *http.Request) {
	var body crmcontracts.ReportingReportInput
	if !httperr.Decode(w, r, &body) {
		return
	}
	out, err := h.service.CreateReport(r.Context(), body)
	created(w, r, out, err)
}

// UpdateReportingReport requires the last observed version to prevent lost edits.
func (h Handlers) UpdateReportingReport(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, params crmcontracts.UpdateReportingReportParams) {
	var body crmcontracts.ReportingReportInput
	if !httperr.Decode(w, r, &body) {
		return
	}
	version, ok := requiredVersion(w, r)
	if !ok {
		return
	}
	out, err := h.service.UpdateReport(r.Context(), ids.UUID(id), version, body)
	respond(w, r, out, err)
}

// CreateReportingTarget records an explicit scoped target through the service.
func (h Handlers) CreateReportingTarget(w http.ResponseWriter, r *http.Request) {
	var body crmcontracts.ReportingTargetInput
	if !httperr.Decode(w, r, &body) {
		return
	}
	out, err := h.service.CreateTarget(r.Context(), body)
	created(w, r, out, err)
}

// UpdateReportingTarget requires the previous version for a reasoned target correction.
func (h Handlers) UpdateReportingTarget(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, params crmcontracts.UpdateReportingTargetParams) {
	var body crmcontracts.ReportingTargetInput
	if !httperr.Decode(w, r, &body) {
		return
	}
	version, ok := requiredVersion(w, r)
	if !ok {
		return
	}
	out, err := h.service.UpdateTarget(r.Context(), ids.UUID(id), version, body)
	respond(w, r, out, err)
}

// PublishReportingFramework requires the current version before appending a stage framework.
func (h Handlers) PublishReportingFramework(w http.ResponseWriter, r *http.Request, params crmcontracts.PublishReportingFrameworkParams) {
	var body crmcontracts.ReportingFrameworkInput
	if !httperr.Decode(w, r, &body) {
		return
	}
	version, ok := requiredVersion(w, r)
	if !ok {
		return
	}
	out, err := h.service.PublishFramework(r.Context(), version, body)
	respond(w, r, out, err)
}

// CreateReportingSchedule binds a recurring capture to a saved report.
func (h Handlers) CreateReportingSchedule(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	var body crmcontracts.ReportingScheduleInput
	if !httperr.Decode(w, r, &body) {
		return
	}
	out, err := h.service.CreateSchedule(r.Context(), ids.UUID(id), body)
	created(w, r, out, err)
}

// UpdateReportingSchedule requires a version before changing publication timing.
func (h Handlers) UpdateReportingSchedule(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, params crmcontracts.UpdateReportingScheduleParams) {
	var body crmcontracts.ReportingScheduleInput
	if !httperr.Decode(w, r, &body) {
		return
	}
	version, ok := requiredVersion(w, r)
	if !ok {
		return
	}
	out, err := h.service.UpdateSchedule(r.Context(), ids.UUID(id), version, body)
	respond(w, r, out, err)
}

// FreezeReportingEdition passes idempotency intent into the shared publication service.
func (h Handlers) FreezeReportingEdition(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, params crmcontracts.FreezeReportingEditionParams) {
	out, err := h.service.Freeze(r.Context(), ids.UUID(id), 0, params.IdempotencyKey)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	w.Header().Set("Location", "/v1/analytics/executions/"+out.Id.String())
	httperr.WriteJSON(w, http.StatusAccepted, out)
}

// PauseReportingSchedules pauses future captures without deleting history.
func (h Handlers) PauseReportingSchedules(w http.ResponseWriter, r *http.Request) {
	count, err := h.service.PauseAll(r.Context())
	respond(w, r, count, err)
}
