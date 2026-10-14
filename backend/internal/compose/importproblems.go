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
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// DownloadImportProblems answers a run's issues as a CSV, through the same
// read and the same refusals as the report. The header and every reason that
// carries a code are written in the language asked for, English by default.
func (h importHandlers) DownloadImportProblems(
	w http.ResponseWriter, r *http.Request, id openapi_types.UUID, params crmcontracts.DownloadImportProblemsParams,
) {
	lang := textlang.English
	if params.Lang != nil {
		lang = textlang.Lang(*params.Lang)
	}
	run, err := h.reported(r, id)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	var buf bytes.Buffer
	if err := writeImportProblems(&buf, toContractImportReport(run), lang); err != nil {
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

// importProblemsHeaders names the problems spreadsheet's columns, in order, per
// language. A language the file is not written in gets the English one.
//
//nolint:goconst // column headings of a downloaded file; the constant goconst offers names an extraction key
var importProblemsHeaders = map[textlang.Lang][]string{
	textlang.English:    {"file", "row", "column", "problem", "value"},
	textlang.German:     {"Datei", "Zeile", "Spalte", "Problem", "Wert"},
	textlang.Vietnamese: {"tệp", "dòng", "cột", "vấn đề", "giá trị"},
}

// problemText is an issue's reason in lang. An issue stored without a code
// keeps the sentence it was written with.
func problemText(issue crmcontracts.ImportRowIssue, lang textlang.Lang) string {
	if issue.ReasonCode == nil {
		return issue.Reason
	}
	var params migration.Params
	if issue.Params != nil {
		params = migration.Params(*issue.Params)
	}
	return migration.Reason{Code: migration.ReasonCode(*issue.ReasonCode), Params: params}.In(lang)
}

// lineText is a problem's row cell, empty for a problem of the whole file.
func lineText(line *int) string {
	if line == nil {
		return ""
	}
	return strconv.Itoa(*line)
}

// writeImportProblems writes a report's issues as a spreadsheet, one line per
// issue. Every cell is the uploader's own text, so each is formula-guarded.
func writeImportProblems(w io.Writer, rep crmcontracts.ImportRunReport, lang textlang.Lang) error {
	out := csv.NewWriter(w)
	if err := out.Write(importProblemsHeaders[lang]); err != nil {
		return fmt.Errorf("import problems: %w", err)
	}
	for _, issue := range rep.Issues {
		row := []string{
			derefString(issue.File), lineText(issue.Line), derefString(issue.Column), problemText(issue, lang), derefString(issue.Value),
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
