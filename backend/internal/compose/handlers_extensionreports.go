// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The extension report directory (GET /v1/extension-reports): the counts and
// breakdowns installed extensions add to Reports and Home, answered for the
// caller.

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// ListExtensionReports is served beside the analytics routes because it needs
// their group floor.
func (h analyticsQueryHandlers) ListExtensionReports(w http.ResponseWriter, r *http.Request) {
	engine := newReportEngine(h.db.Pool())
	data := extensionReportDirectory(r.Context(), registeredReports(),
		func(ctx context.Context, report registeredReport) (extensionReportAnswer, bool, error) {
			return engine.runRegistered(ctx, report, h.floor)
		})
	httperr.WriteJSON(w, http.StatusOK, crmcontracts.ExtensionReportDirectory{Data: data})
}

// extensionReportDirectory runs each report on its own. A report the caller may
// not read is left out, so no one learns it exists. A report that fails is
// listed as unavailable and the rest still answer. Each run opens its own
// transactions, so one failure cannot poison the next.
func extensionReportDirectory(
	ctx context.Context, reports []registeredReport,
	run func(context.Context, registeredReport) (extensionReportAnswer, bool, error),
) []crmcontracts.ExtensionReport {
	data := []crmcontracts.ExtensionReport{}
	for _, report := range reports {
		answer, ok, err := run(ctx, report)
		switch {
		case errors.Is(err, apperrors.ErrPermissionDenied) || (err == nil && !ok):
			continue
		case err != nil:
			slog.ErrorContext(ctx, "extension report could not run",
				"unit", string(report.unit), "report", report.name, "err", err)
			data = append(data, reportHeadWire(report, crmcontracts.ExtensionReportStatusUnavailable))
		default:
			data = append(data, extensionReportWire(answer))
		}
	}
	return data
}

// reportHeadWire is what every state of a report says about it. Alone it is an
// unavailable report: no groups, no total, nothing withheld.
func reportHeadWire(report registeredReport, status crmcontracts.ExtensionReportStatus) crmcontracts.ExtensionReport {
	out := crmcontracts.ExtensionReport{
		Name: report.name, Unit: string(report.unit), Label: report.label,
		Kind:       crmcontracts.ExtensionReportKind(report.kind),
		Population: report.population, Status: status,
		Groups: []crmcontracts.ExtensionReportGroup{},
	}
	if report.by != "" {
		out.By = &report.by
	}
	return out
}

func extensionReportWire(a extensionReportAnswer) crmcontracts.ExtensionReport {
	out := reportHeadWire(a.Report, crmcontracts.ExtensionReportStatusOk)
	out.Withheld = a.Withheld
	out.Total, out.ExcludedByPermission = a.Total, a.ExcludedByPermission
	for _, g := range a.Groups {
		group := crmcontracts.ExtensionReportGroup{Withheld: g.Withheld}
		if !g.Withheld {
			count := g.Count
			group.Count = &count
			if key, ok := g.Key.(string); ok {
				group.Key = &key
			}
		}
		out.Groups = append(out.Groups, group)
	}
	return out
}
