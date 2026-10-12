// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package reporting

import (
	"net/http"
	"net/url"
	"strings"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func respond[T any](w http.ResponseWriter, r *http.Request, value T, err error) {
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, value)
}

func requiredVersion(w http.ResponseWriter, r *http.Request) (int64, bool) {
	version, ok := httperr.IfMatchVersion(w, r)
	if !ok {
		return 0, false
	}
	if version == nil {
		httperr.Write(w, r, invalid("send the last seen version in If-Match"))
		return 0, false
	}
	return *version, true
}

//nolint:nilnil // A missing cursor denotes the first page.
func pageID(cursor *string) (*ids.UUID, error) {
	if cursor == nil {
		return nil, nil
	}
	id, err := ids.Parse(*cursor)
	if err != nil {
		return nil, &storekit.MalformedCursorError{}
	}
	return &id, nil
}

func pageLimit(value *int) int {
	if value == nil {
		return 50
	}
	return *value
}

func textOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

// EvaluateReporting uses the same selection parser as evidence reads.
func (h Handlers) EvaluateReporting(w http.ResponseWriter, r *http.Request, _ crmcontracts.EvaluateReportingParams) {
	selection, err := SelectionFromQuery(r.URL.Query())
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	out, err := h.service.Evaluate(r.Context(), selection)
	respond(w, r, out, err)
}

// GetReportingEvidence checks the live evaluation receipt before returning source rows.
func (h Handlers) GetReportingEvidence(w http.ResponseWriter, r *http.Request, params crmcontracts.GetReportingEvidenceParams) {
	selection, err := SelectionFromQuery(r.URL.Query())
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	out, err := h.service.Evidence(r.Context(), selection, params.Metric, params.ContextId, textOrEmpty(params.GroupKey), params.Through, params.Cursor, pageLimit(params.Limit), EvidenceExpectation{Key: params.EvaluationKey, At: params.EvaluatedAt, FrameworkRevision: params.FrameworkRevision})
	respond(w, r, out, err)
}

// GetReportingEditionEvidence reads frozen contributions under current permissions.
func (h Handlers) GetReportingEditionEvidence(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, params crmcontracts.GetReportingEditionEvidenceParams) {
	out, err := h.service.EditionEvidence(r.Context(), ids.UUID(id), params.Metric, params.ContextId, textOrEmpty(params.GroupKey), params.Through, params.Cursor, pageLimit(params.Limit))
	respond(w, r, out, err)
}

// SelectionFromQuery gives live views and evidence identical filter parsing.
func SelectionFromQuery(q url.Values) (crmcontracts.ReportingSelection, error) {
	selection := crmcontracts.ReportingSelection{Period: reportingThisMonth, TargetBasis: "month", CloseWindow: reportingFiscalQuarter, Metrics: []crmcontracts.ReportingMetricID{"bookings_won", "open_pipeline", reportingStageAge}, Blocks: []crmcontracts.ReportingBlockKind{"bookings_trend", "stage_distribution", "owner_attainment", reportingStageAge}}
	selection.Scope.Kind = crmcontracts.ReportingScopeKind(q.Get("scope_kind"))
	for _, binding := range []struct {
		name        string
		destination **openapi_types.UUID
	}{{"scope_id", &selection.Scope.Id}, {"pipeline_id", &selection.PipelineId}} {
		if value := q.Get(binding.name); value != "" {
			id, err := ids.Parse(value)
			if err != nil {
				return selection, invalid("choose a valid reporting scope and pipeline")
			}
			wire := openapi_types.UUID(id)
			*binding.destination = &wire
		}
	}
	if value := q.Get("period"); value != "" {
		selection.Period = crmcontracts.ReportingSelectionPeriod(value)
	}
	if value := q.Get("target_basis"); value != "" {
		selection.TargetBasis = crmcontracts.ReportingSelectionTargetBasis(value)
	}
	if value := q.Get("close_window"); value != "" {
		selection.CloseWindow = crmcontracts.ReportingSelectionCloseWindow(value)
	}
	if q.Has("start_at") || q.Has("end_at") {
		start, err := time.Parse(time.RFC3339Nano, q.Get("start_at"))
		if err != nil {
			return selection, invalid("choose a valid reporting start")
		}
		end, err := time.Parse(time.RFC3339Nano, q.Get("end_at"))
		if err != nil {
			return selection, invalid("choose a valid reporting end")
		}
		selection.Interval = &crmcontracts.ReportingWindow{StartAt: start, EndAt: end}
	}
	if q.Has("metrics") {
		selection.Metrics = nil
		for _, value := range queryItems(q["metrics"]) {
			selection.Metrics = append(selection.Metrics, crmcontracts.ReportingMetricID(value))
		}
		// The default charts belong to the default metrics; paired with metrics
		// the caller chose they fail the "charts belong to the metrics" rule.
		if !q.Has("blocks") {
			selection.Blocks = []crmcontracts.ReportingBlockKind{}
		}
	}
	if q.Has("blocks") {
		selection.Blocks = nil
		for _, value := range queryItems(q["blocks"]) {
			selection.Blocks = append(selection.Blocks, crmcontracts.ReportingBlockKind(value))
		}
	}
	return selection, nil
}

func created[T any](w http.ResponseWriter, r *http.Request, value T, err error) {
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusCreated, value)
}

func queryItems(values []string) []string {
	var out []string
	for _, value := range values {
		out = append(out, strings.Split(value, ",")...)
	}
	return out
}
