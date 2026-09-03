// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The import surface's wire half: what a request must satisfy before a run
// exists, what one uploaded part yields, and how a run and its report reach
// the contract's shapes. Kept beside the handlers rather than inside them so
// each door stays readable as a sequence of decisions.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"strings"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// mappingFrom validates the requested mapping against the object's live field
// catalog. A target the object does not have is refused here rather than at row
// 40,000 of a commit.
func mappingFrom(object string, req crmcontracts.CreateImportRunRequest) (migration.RunMapping, error) {
	targets, err := importTargets(object)
	if err != nil {
		return migration.RunMapping{}, err
	}
	allowed := make(map[string]bool, len(targets))
	for _, t := range targets {
		allowed[t] = true
	}
	fields := map[string]string{}
	claimed := map[string]string{}
	for column, target := range req.Mapping {
		if !allowed[target] {
			// The set is closed, small, and invisible from where the caller
			// stands. Naming the target that failed without naming the ones
			// that would have worked leaves them guessing at a list this
			// function is already holding. `city` is the case that forces it:
			// it reads like an obvious field on a company, is not one, and the
			// refusal alone gives no path to the answer.
			return migration.RunMapping{}, httperr.Validation("mapping", "unknown_target",
				fmt.Sprintf("%q is not a field a %s can receive. It takes: %s.",
					target, object, strings.Join(targets, ", ")))
		}
		if other, taken := claimed[target]; taken {
			// Two columns onto one field: whichever the row builder happened to
			// write last would win, so identical requests could import
			// different data. Refused rather than resolved by luck.
			return migration.RunMapping{}, httperr.Validation("mapping", "duplicate_target",
				fmt.Sprintf("%q and %q both map to %q; one column per field.", other, column, target))
		}
		claimed[target] = column
		fields[column] = target
	}
	if len(fields) == 0 {
		// Reached two ways that feel different to a caller: they sent no
		// mapping and the proposal came back empty, or they sent one whose
		// columns all fell away. Either way the next move is the same and it
		// needs the vocabulary, because a header like "Company,Website,City"
		// matches none of these by name and the proposal is deliberately
		// timid about guessing (migration.SuggestMapping).
		return migration.RunMapping{}, httperr.Validation("mapping", "empty",
			fmt.Sprintf("A run with no mapped column would import nothing. "+
				"Map at least one column onto a field a %s can receive: %s.",
				object, strings.Join(targets, ", ")))
	}

	sourceKey := csvSourceKeyDefault[object]
	if req.SourceKey != nil && strings.TrimSpace(*req.SourceKey) != "" {
		sourceKey = strings.TrimSpace(*req.SourceKey)
	} else if idColumn := columnFor(fields, csvTargetID); idColumn != "" &&
		columnFor(fields, csvSourceKeyDefault[object]) == "" {
		// A file that names its records and carries nothing else to identify a
		// row by falls back to the id column. That is what lets a corrections
		// export be "id,city" — a whole legitimate file, which defaulting to
		// display_name would refuse for not carrying a name it was never going
		// to change.
		//
		// It is a FALLBACK, not a preference: every row then needs an id, since
		// a row with no source key cannot be identified for re-import or undo.
		// A file mixing corrections with new companies therefore keeps its own
		// key column, and the id column says which rows are corrections.
		sourceKey = idColumn
	} else {
		// The default names a TARGET field; the source must name the column
		// mapped onto it, or no row can be identified.
		sourceKey = columnFor(fields, sourceKey)
	}
	if sourceKey == "" {
		return migration.RunMapping{}, httperr.Validation("source_key", "unmappable",
			fmt.Sprintf("Map a column to %q, or name the column that identifies a row.", csvSourceKeyDefault[object]))
	}
	if _, mapped := fields[sourceKey]; !mapped {
		return migration.RunMapping{}, httperr.Validation("source_key", "unmapped_column",
			fmt.Sprintf("%q is not one of the mapped columns.", sourceKey))
	}
	onDuplicate := string(crmcontracts.Create)
	if req.OnDuplicate != nil {
		if !req.OnDuplicate.Valid() {
			return migration.RunMapping{}, httperr.Validation("on_duplicate", "invalid_enum",
				fmt.Sprintf("%q is not a duplicate policy; it takes create or skip.", string(*req.OnDuplicate)))
		}
		onDuplicate = string(*req.OnDuplicate)
	}
	contextTag := ""
	if req.ContextTagId != nil {
		// Carried as text on the mapping, and parsed back where it is applied.
		// The zero UUID is not a tag: a body sending one names no word, and
		// storing it would make every created record fail its apply at commit
		// time — long after the request that could have said so.
		if ids.UUID(*req.ContextTagId).IsZero() {
			return migration.RunMapping{}, httperr.Validation("context_tag_id", "empty",
				"A context tag names an existing tag by id. Leave it out to file this run under no word.")
		}
		contextTag = req.ContextTagId.String()
	}
	return migration.RunMapping{
		Object: object, Fields: fields, SourceKey: sourceKey, OnDuplicate: onDuplicate,
		ContextTag: contextTag,
	}, nil
}

// columnFor answers which source column was mapped onto a target field.
func columnFor(fields map[string]string, target string) string {
	for column, mapped := range fields {
		if mapped == target {
			return column
		}
	}
	return ""
}

// importProblem keeps a file's own failures on the caller's side of the line:
// an unreadable upload or an unusable header is the customer's file, not our
// server, and it is told which so they can fix it.
func importProblem(err error) error {
	switch {
	case errors.Is(err, migration.ErrHeaderInvalid):
		return httperr.Validation("file", "header_unusable", err.Error())
	case errors.Is(err, migration.ErrSourceUnreadable):
		return httperr.Validation("file", "unreadable", err.Error())
	case errors.Is(err, migration.ErrObjectNotInSource):
		return httperr.Validation("object", "mismatch", err.Error())
	case errors.Is(err, blobstore.ErrNotFound):
		// The reference was this workspace's (ownsSource proved that) but the
		// bytes are gone. Not-found names what is actually missing; a 500 would
		// blame the server for a source the caller can simply re-upload.
		return fmt.Errorf("the uploaded file behind this run is no longer stored: %w", apperrors.ErrNotFound)
	default:
		return err
	}
}

// withSkippedLines folds the source's own disclosures into the object's report:
// a row the source could not deliver never reached the writer, so nothing else
// in the report would ever mention it.
func withSkippedLines(report migration.Report, object string, skipped []migration.SkippedLine) migration.Report {
	if len(skipped) == 0 {
		return report
	}
	for i := range report.Objects {
		if report.Objects[i].Object != object {
			continue
		}
		for _, s := range skipped {
			report.Objects[i].Skipped = append(report.Objects[i].Skipped, migration.SkippedRow{
				ExternalID: fmt.Sprintf("line %d", s.Line),
				Line:       s.Line,
				Reason:     s.Reason,
			})
		}
	}
	return report
}

func toContractColumns(p migration.Profile) []crmcontracts.ImportColumn {
	out := make([]crmcontracts.ImportColumn, 0, len(p.Columns))
	for _, c := range p.Columns {
		samples := c.Samples
		if samples == nil {
			samples = []string{}
		}
		out = append(out, crmcontracts.ImportColumn{Header: c.Header, FillRate: float32(c.FillRate), Samples: samples})
	}
	return out
}

func toContractImportRun(run migration.Run) crmcontracts.ImportRun {
	out := crmcontracts.ImportRun{
		Id:         openapi_types.UUID(run.ID),
		Connector:  crmcontracts.ImportRunConnector(run.Connector),
		Status:     crmcontracts.ImportRunStatus(run.Status),
		Checkpoint: run.Checkpoint,
		Source:     importSourceProvenance,
		CapturedBy: &run.CapturedBy,
		CreatedAt:  run.CreatedAt,
		UpdatedAt:  run.UpdatedAt,
	}
	if run.Mapping != nil {
		out.Object = crmcontracts.ImportObject(run.Mapping.Object)
	}
	if run.Error != "" {
		message := run.Error
		out.Error = &message
	}
	return out
}

// toContractUndoReport renders the reversal outcome (IEM-WIRE-9).
func toContractUndoReport(id migration.RunID, status string, rep migration.UndoReport) *crmcontracts.ImportUndoReport {
	kept := make([]struct {
		Id     openapi_types.UUID        `json:"id"` //nolint:staticcheck // matches the generated ImportUndoReport.Kept item shape
		Object crmcontracts.ImportObject `json:"object"`
	}, 0, len(rep.Kept))
	for _, k := range rep.Kept {
		kept = append(kept, struct {
			Id     openapi_types.UUID        `json:"id"` //nolint:staticcheck // matches the generated ImportUndoReport.Kept item shape
			Object crmcontracts.ImportObject `json:"object"`
		}{Id: openapi_types.UUID(k.ID), Object: crmcontracts.ImportObject(k.Object)})
	}
	errored := make([]struct {
		Id     openapi_types.UUID        `json:"id"` //nolint:staticcheck // matches the generated ImportUndoReport.Errored item shape
		Object crmcontracts.ImportObject `json:"object"`
		Reason string                    `json:"reason"`
	}, 0, len(rep.Errored))
	for _, e := range rep.Errored {
		errored = append(errored, struct {
			Id     openapi_types.UUID        `json:"id"` //nolint:staticcheck // matches the generated ImportUndoReport.Errored item shape
			Object crmcontracts.ImportObject `json:"object"`
			Reason string                    `json:"reason"`
		}{Id: openapi_types.UUID(e.ID), Object: crmcontracts.ImportObject(e.Object), Reason: e.Reason})
	}
	return &crmcontracts.ImportUndoReport{
		RunId:         openapi_types.UUID(id),
		Status:        crmcontracts.ImportRunStatus(status),
		ReversedCount: rep.ReversedCount,
		Kept:          kept,
		Errored:       errored,
	}
}

// readImportUpload takes the multipart body apart under the deployment's import
// cap and returns the object the rows are and the file's bytes.
//
// The bytes are read whole rather than streamed to the blobstore, and that is
// deliberate: the same upload must be BOTH profiled and stored, and a stream
// can only be consumed once. The cap is what makes reading it whole safe, and
// MaxBytesReader (not a bare LimitReader) is what turns an over-cap upload into
// a refusal rather than a file silently cut in half.
func readImportUpload(w http.ResponseWriter, r *http.Request, limit int64) (string, []byte, error) {
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	// upload:route /v1/imports/sources — the ceiling this parse runs under is granted to that
	// path in compose.uploadCeilings, and TestEveryMultipartParseNamesItsRoute
	// holds the two together.
	//nolint:gosec // G120 wants a bound here, and the bound is the MaxBytesReader above: this argument is only the in-memory/spill threshold, and it is deliberately far below the ceiling so the parse spills rather than holding the upload resident.
	if err := r.ParseMultipartForm(importSpillBytes); err != nil {
		// The cap is DERIVED, never spelled again in prose: this sentence said
		// "10 MB" while the constant beside it decided the real answer, and the
		// two were free to drift the moment either moved.
		return "", nil, httperr.MultipartRefusal(err, limit)
	}

	object := strings.TrimSpace(r.FormValue("object"))
	if _, ok := csvTargets[object]; !ok {
		return "", nil, httperr.Validation("object", "unsupported",
			"An import lands organizations, people or leads; name one of them.")
	}

	file, _, err := r.FormFile("file")
	if err != nil {
		return "", nil, httperr.Validation("file", "missing", "Attach the file to import as `file`.")
	}
	defer func(ctx context.Context) {
		if cerr := file.Close(); cerr != nil {
			slog.WarnContext(ctx, "closing the uploaded import part", "err", cerr)
		}
	}(r.Context())

	body, err := readAllUpload(file)
	if err != nil {
		return "", nil, httperr.Validation("file", "unreadable", "The upload could not be read.")
	}
	if len(body) == 0 {
		return "", nil, httperr.Validation("file", "empty", "The uploaded file has no content.")
	}
	return object, body, nil
}

func readAllUpload(file multipart.File) ([]byte, error) {
	body, err := io.ReadAll(file)
	if err != nil {
		return nil, fmt.Errorf("reading the uploaded file: %w", err)
	}
	return body, nil
}

// refinePrediction replaces the engine's create/update split with one the
// report can honestly show a human.
//
// The engine classifies from Writers.Exists alone, so every row that already
// landed counts as an update — including the ones whose values are identical,
// which the commit will not rewrite. A dry run whose whole job is to say what
// WILL happen may not overstate it by the size of the customer's re-upload, so
// each row is compared here exactly as the commit will compare it.
