// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"net/http"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/httperr"
)

// The contract's bounds for `GET /imports?limit=`.
const (
	importRunListDefault = 20
	importRunListMax     = 50
)

// ListImportRuns answers the newest CSV runs for the Data import page. The
// store takes the import_run read grant before it touches the database.
func (h importHandlers) ListImportRuns(w http.ResponseWriter, r *http.Request, params crmcontracts.ListImportRunsParams) {
	runs, err := migration.NewRunStore(h.db).ListStaged(r.Context(), importRunListLimit(params.Limit))
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	out := crmcontracts.ImportRunList{Data: make([]crmcontracts.ImportRun, 0, len(runs))}
	for _, run := range runs {
		out.Data = append(out.Data, toContractImportRun(run))
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

// importRunListLimit holds a requested page size to the contract's bounds, as
// storekit.ClampLimit does for the default list size.
func importRunListLimit(limit *int) int {
	if limit == nil {
		return importRunListDefault
	}
	return min(max(*limit, 1), importRunListMax)
}
