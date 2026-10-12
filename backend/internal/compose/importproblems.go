// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"strconv"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// DownloadImportProblems answers a run's issues as a CSV, through the same
// read and the same refusals as the report.
func (h importHandlers) DownloadImportProblems(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	run, err := h.reported(r, id)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	var buf bytes.Buffer
	if err := writeImportProblems(&buf, toContractImportReport(run)); err != nil {
		httperr.Write(w, r, err)
		return
	}
	writeImportDownload(w, r, httperr.Download{ContentType: reportingCSVContentType, Filename: "problems.csv"}, buf.Bytes())
}

// reported is a run that has a report: one a dry run has validated.
func (h importHandlers) reported(r *http.Request, id openapi_types.UUID) (migration.Run, error) {
	run, err := h.staged(r, id)
	if err != nil {
		return migration.Run{}, err
	}
	if run.Report == nil {
		return migration.Run{}, fmt.Errorf("import run %s has not been validated yet: %w", run.ID, apperrors.ErrConflict)
	}
	return run, nil
}

// importProblemsHeader names the problems spreadsheet's columns, in order.
//
//nolint:goconst // column headings of a downloaded file; the constant goconst offers names an extraction key
var importProblemsHeader = []string{"file", "row", "column", "problem", "value"}

// writeImportProblems writes a report's issues as a spreadsheet, one line per
// issue. Every cell is the uploader's own text, so each is formula-guarded.
func writeImportProblems(w io.Writer, rep crmcontracts.ImportRunReport) error {
	out := csv.NewWriter(w)
	if err := out.Write(importProblemsHeader); err != nil {
		return fmt.Errorf("import problems: %w", err)
	}
	for _, issue := range rep.Issues {
		row := []string{
			derefString(issue.File), strconv.Itoa(issue.Line), derefString(issue.Column), issue.Reason, derefString(issue.Value),
		}
		for i := range row {
			row[i] = guardCSVFormula(row[i])
		}
		if err := out.Write(row); err != nil {
			return fmt.Errorf("import problems: %w", err)
		}
	}
	out.Flush()
	return out.Error()
}
