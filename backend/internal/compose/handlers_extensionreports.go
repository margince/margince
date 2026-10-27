// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The extension report directory (GET /v1/extension-reports): the counts and
// breakdowns installed extensions add to Reports and Home, answered for the
// caller.

import (
	"errors"
	"net/http"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// ListExtensionReports is served beside the analytics routes because it needs
// their group floor. A report the caller may not read is left out, as an
// unreadable record kind is from the kind directory.
func (h analyticsQueryHandlers) ListExtensionReports(w http.ResponseWriter, r *http.Request) {
	engine := newReportEngine(h.db.Pool())
	data := []crmcontracts.ExtensionReport{}
	for _, report := range registeredReports() {
		answer, ok, err := engine.runRegistered(r.Context(), report, h.floor)
		if errors.Is(err, apperrors.ErrPermissionDenied) || (err == nil && !ok) {
			continue
		}
		if err != nil {
			httperr.Write(w, r, err)
			return
		}
		data = append(data, extensionReportWire(answer))
	}
	httperr.WriteJSON(w, http.StatusOK, crmcontracts.ExtensionReportDirectory{Data: data})
}

func extensionReportWire(a extensionReportAnswer) crmcontracts.ExtensionReport {
	out := crmcontracts.ExtensionReport{
		Name: a.Report.name, Unit: string(a.Report.unit), Label: a.Report.label,
		Kind:       crmcontracts.ExtensionReportKind(a.Report.kind),
		Population: a.Report.population, Withheld: a.Withheld,
		Total: a.Total, ExcludedByPermission: a.ExcludedByPermission,
		Groups: make([]crmcontracts.ExtensionReportGroup, 0, len(a.Groups)),
	}
	if a.Report.by != "" {
		out.By = &a.Report.by
	}
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
