// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package reporting

import (
	"net/http"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Handlers adapts generated HTTP contracts to the shared reporting service.
type Handlers struct{ service *Service }

// NewHandlers keeps transport parsing outside reporting calculations.
func NewHandlers(service *Service) Handlers { return Handlers{service: service} }

// GetReportingMetrics returns permission-filtered metric definitions.
func (h Handlers) GetReportingMetrics(w http.ResponseWriter, r *http.Request) {
	out, err := h.service.Catalog(r.Context())
	respond(w, r, out, err)
}

// GetReportingFramework returns the stage framework used by all reporting surfaces.
func (h Handlers) GetReportingFramework(w http.ResponseWriter, r *http.Request) {
	out, err := h.service.GetFramework(r.Context())
	respond(w, r, out, err)
}

// GetReportingReport returns a report only through its current audience.
func (h Handlers) GetReportingReport(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	out, err := h.service.GetReport(r.Context(), ids.UUID(id))
	respond(w, r, out, err)
}

// ArchiveReportingReport archives through the service so schedules pause in the same transaction.
func (h Handlers) ArchiveReportingReport(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	out, err := h.service.ArchiveReport(r.Context(), ids.UUID(id))
	respond(w, r, out, err)
}

// GetReportingTarget checks target scope through the shared service.
func (h Handlers) GetReportingTarget(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	out, err := h.service.GetTarget(r.Context(), ids.UUID(id))
	respond(w, r, out, err)
}

// GetReportingEdition projects frozen data through current disclosure authority.
func (h Handlers) GetReportingEdition(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	out, err := h.service.GetEdition(r.Context(), ids.UUID(id))
	respond(w, r, out, err)
}

// GetReportingExecution returns publication status through report audience checks.
func (h Handlers) GetReportingExecution(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	out, err := h.service.GetExecution(r.Context(), ids.UUID(id))
	respond(w, r, out, err)
}

// RetryReportingExecution requests a bounded retry of the original publication intent.
func (h Handlers) RetryReportingExecution(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	out, err := h.service.RetryExecution(r.Context(), ids.UUID(id))
	respond(w, r, out, err)
}

// ListReportingSchedules checks report visibility before listing schedules.
func (h Handlers) ListReportingSchedules(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	out, err := h.service.ListSchedules(r.Context(), ids.UUID(id))
	respond(w, r, out, err)
}

// EvaluateReportingReport evaluates the selected saved revision with current source data.
func (h Handlers) EvaluateReportingReport(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, params crmcontracts.EvaluateReportingReportParams) {
	out, err := h.service.EvaluateReport(r.Context(), ids.UUID(id), params.Revision)
	respond(w, r, out, err)
}

// CompareReportingEditions refuses unsupported period or population comparisons.
func (h Handlers) CompareReportingEditions(w http.ResponseWriter, r *http.Request, params crmcontracts.CompareReportingEditionsParams) {
	out, err := h.service.Compare(r.Context(), ids.UUID(params.LeftId), ids.UUID(params.RightId))
	respond(w, r, out, err)
}

// ListReportingReports keeps cursor validation at the transport boundary.
func (h Handlers) ListReportingReports(w http.ResponseWriter, r *http.Request, params crmcontracts.ListReportingReportsParams) {
	after, err := pageID(params.Cursor)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	out, err := h.service.ListReports(r.Context(), after, pageLimit(params.Limit), params.Scheduled != nil && *params.Scheduled)
	respond(w, r, out, err)
}

// ListReportingTargets keeps target pagination within caller scope.
func (h Handlers) ListReportingTargets(w http.ResponseWriter, r *http.Request, params crmcontracts.ListReportingTargetsParams) {
	after, err := pageID(params.Cursor)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	out, err := h.service.ListTargets(r.Context(), after, pageLimit(params.Limit), TargetFilter{Retired: params.Retired, PeriodStart: params.PeriodStart})
	respond(w, r, out, err)
}

// ListReportingEditions bounds expensive current-authority projections per page.
func (h Handlers) ListReportingEditions(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, params crmcontracts.ListReportingEditionsParams) {
	after, err := pageID(params.Cursor)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	out, err := h.service.ListEditions(r.Context(), ids.UUID(id), after, pageLimit(params.Limit))
	respond(w, r, out, err)
}

// ListReportingExecutions paginates status through the owning report.
func (h Handlers) ListReportingExecutions(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, params crmcontracts.ListReportingExecutionsParams) {
	after, err := pageID(params.Cursor)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	out, err := h.service.ListExecutions(r.Context(), ids.UUID(id), after, pageLimit(params.Limit))
	respond(w, r, out, err)
}
