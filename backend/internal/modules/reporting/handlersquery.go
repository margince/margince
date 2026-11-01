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

// unpublishedFramework is the version GET /analytics/framework answers before
// the first publish, so that publish must be able to name it.
const unpublishedFramework = "0"

// frameworkVersion is requiredVersion that also takes the unpublished version.
func frameworkVersion(w http.ResponseWriter, r *http.Request) (int64, bool) {
	if r.Header.Get("If-Match") == unpublishedFramework {
		return 0, true
	}
	return requiredVersion(w, r)
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
	selection, err := SelectionFromQuery(r.URL.Query(), h.opening)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	out, err := h.service.Evaluate(r.Context(), selection)
	respond(w, r, out, err)
}

// GetReportingEvidence checks the live evaluation receipt before returning source rows.
func (h Handlers) GetReportingEvidence(w http.ResponseWriter, r *http.Request, params crmcontracts.GetReportingEvidenceParams) {
	selection, err := SelectionFromQuery(r.URL.Query(), h.opening)
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

// OpeningSelection is what a request that names no metrics opens on.
//
// Passed in rather than spelled here. The catalog publishes it as a preset,
// which the Analytics screen and the daily benchmark read. A second copy here
// drifted from it by two metrics.
type OpeningSelection struct {
	Metrics []crmcontracts.ReportingMetricID
	Blocks  []crmcontracts.ReportingBlockKind
}

// SelectionFromQuery gives live views and evidence identical filter parsing.
func SelectionFromQuery(q url.Values, opening OpeningSelection) (crmcontracts.ReportingSelection, error) {
	selection := crmcontracts.ReportingSelection{
		Period: reportingThisMonth, TargetBasis: "month", CloseWindow: reportingFiscalQuarter,
		Metrics: opening.Metrics, Blocks: opening.Blocks,
	}
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
	chooseMembers(q, &selection)
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

// chooseMembers replaces the opening selection's metrics and charts with the
// ones the query names.
//
// Metrics named alone carry no charts. The opening's charts belong to the
// opening's metrics. Drawing them over metrics the caller chose breaks the
// rule that a chart belongs to what it draws.
func chooseMembers(q url.Values, selection *crmcontracts.ReportingSelection) {
	if q.Has("metrics") {
		selection.Metrics = nil
		for _, value := range queryItems(q["metrics"]) {
			selection.Metrics = append(selection.Metrics, crmcontracts.ReportingMetricID(value))
		}
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
}
