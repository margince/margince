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
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// stagedMappingFrom turns a create request into the mapping a run is staged
// with. A single CSV sends object and mapping, and a ZIP sends one part per
// file. A request with both shapes or neither is refused.
func stagedMappingFrom(req crmcontracts.CreateImportRunRequest) (migration.RunMapping, error) {
	single := req.Object != nil || req.Mapping != nil
	if single == (req.Parts != nil) || (req.Parts != nil && req.SourceKey != nil) {
		return migration.RunMapping{}, httperr.Validation("parts", "one_shape",
			"Send either object, mapping and source_key for one CSV, or parts for a ZIP.")
	}
	m, err := runOptionsFrom(req)
	if err != nil {
		return migration.RunMapping{}, err
	}
	if single {
		if req.Object == nil || req.Mapping == nil {
			return migration.RunMapping{}, httperr.Validation("mapping", "one_shape",
				"A single CSV needs both object and mapping.")
		}
		part, err := partMappingFrom(string(*req.Object), *req.Mapping, req.SourceKey)
		if err != nil {
			return migration.RunMapping{}, err
		}
		m.Object, m.Fields, m.SourceKey = part.Object, part.Fields, part.SourceKey
		return m, nil
	}
	m.Parts, err = partsMappingFrom(*req.Parts)
	if err != nil {
		return migration.RunMapping{}, err
	}
	return m, nil
}

// partsMappingFrom validates a ZIP's parts: each names its own file and its own
// object, and each mapping holds as a single CSV's would.
func partsMappingFrom(requested []crmcontracts.ImportPartRequest) ([]migration.PartMapping, error) {
	if len(requested) == 0 {
		return nil, httperr.Validation("parts", "empty", "Name at least one file of the ZIP to import.")
	}
	fileOf, named := map[string]string{}, map[string]bool{}
	parts := make([]migration.PartMapping, 0, len(requested))
	for _, p := range requested {
		if strings.TrimSpace(p.File) == "" || named[p.File] {
			return nil, httperr.Validation("parts", "file_name",
				fmt.Sprintf("Each part names a different file of the ZIP; %q is blank or named twice.", p.File))
		}
		named[p.File] = true
		object := string(p.Object)
		if other, taken := fileOf[object]; taken {
			// Two files of one type would land under one source key namespace,
			// and which file's row won a shared id would be read order.
			return nil, httperr.Validation("parts", "duplicate_object",
				fmt.Sprintf("%q and %q are both %s files; a ZIP imports one file per type.", other, p.File, object))
		}
		fileOf[object] = p.File
		part, err := partMappingFrom(object, p.Mapping, p.SourceKey)
		if err != nil {
			return nil, err
		}
		part.File = p.File
		parts = append(parts, part)
	}
	return parts, nil
}

// sourceNamePattern is the contract's pattern for `source_name`.
var sourceNamePattern = regexp.MustCompile(`^[A-Za-z0-9 ._-]*$`)

const (
	maxSourceName     = 80
	maxImportFileName = 255
)

// sourceNameFrom trims the requested source name and holds it to the contract.
func sourceNameFrom(raw *string) (string, error) {
	if raw == nil {
		return "", nil
	}
	name := strings.TrimSpace(*raw)
	if len(name) > maxSourceName || !sourceNamePattern.MatchString(name) {
		return "", httperr.Validation("source_name", "invalid",
			fmt.Sprintf("A source name is at most %d letters, digits, spaces, dots, underscores or hyphens.", maxSourceName))
	}
	return name, nil
}

// unsupportedObject refuses an object no import lands.
func unsupportedObject() error {
	return httperr.Validation("object", "unsupported",
		"An import lands companies, contacts or leads; name one of them.")
}

// partMappingFrom validates one file's mapping against the object's live field
// catalog. A target the object does not have is refused here rather than at row
// 40,000 of a commit.
func partMappingFrom(object string, mapping map[string]string, requestedKey *string) (migration.PartMapping, error) {
	targets, err := importTargets(object)
	if err != nil {
		return migration.PartMapping{}, unsupportedObject()
	}
	allowed := make(map[string]bool, len(targets))
	for _, t := range targets {
		allowed[t] = true
	}
	fields := map[string]string{}
	claimed := map[string]string{}
	for column, target := range mapping {
		if !allowed[target] {
			// The set is closed, small, and invisible from where the caller
			// stands. Naming the target that failed without naming the ones
			// that would have worked leaves them guessing at a list this
			// function is already holding. `city` is the case that forces it:
			// it reads like an obvious field on a company, is not one, and the
			// refusal alone gives no path to the answer.
			return migration.PartMapping{}, httperr.Validation("mapping", "unknown_target",
				fmt.Sprintf("%q is not a field a %s can receive. It takes: %s.",
					target, object, strings.Join(targets, ", ")))
		}
		if other, taken := claimed[target]; taken {
			// Two columns onto one field: whichever the row builder happened to
			// write last would win, so identical requests could import
			// different data. Refused rather than resolved by luck.
			return migration.PartMapping{}, httperr.Validation("mapping", "duplicate_target",
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
		return migration.PartMapping{}, httperr.Validation("mapping", "empty",
			fmt.Sprintf("A run with no mapped column would import nothing. "+
				"Map at least one column onto a field a %s can receive: %s.",
				object, strings.Join(targets, ", ")))
	}

	sourceKey := csvSourceKeyDefault[object]
	if refColumn := columnFor(fields, csvTargetRef); refColumn != "" {
		// Other files' links name the source system's own row id. A re-upload
		// and an undo find the row by it too.
		sourceKey = refColumn
	} else if requestedKey != nil && strings.TrimSpace(*requestedKey) != "" {
		sourceKey = strings.TrimSpace(*requestedKey)
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
		return migration.PartMapping{}, httperr.Validation("source_key", "unmappable",
			fmt.Sprintf("Map a column to %q, or name the column that identifies a row.", csvSourceKeyDefault[object]))
	}
	if _, mapped := fields[sourceKey]; !mapped {
		return migration.PartMapping{}, httperr.Validation("source_key", "unmapped_column",
			fmt.Sprintf("%q is not one of the mapped columns.", sourceKey))
	}
	return migration.PartMapping{Object: object, Fields: fields, SourceKey: sourceKey}, nil
}

// runOptionsFrom reads what a request decides for the whole run, whichever of
// the two shapes it takes.
func runOptionsFrom(req crmcontracts.CreateImportRunRequest) (migration.RunMapping, error) {
	onDuplicate := string(crmcontracts.ImportOnDuplicateCreate)
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
	onBrokenLink := string(crmcontracts.ImportOnBrokenLinkSkipRow)
	if req.OnBrokenLink != nil {
		if !req.OnBrokenLink.Valid() {
			return migration.RunMapping{}, httperr.Validation("on_broken_link", "invalid_enum",
				fmt.Sprintf("%q is not a broken-link policy; it takes skip_row or drop_link.", string(*req.OnBrokenLink)))
		}
		onBrokenLink = string(*req.OnBrokenLink)
	}
	sourceName, err := sourceNameFrom(req.SourceName)
	if err != nil {
		return migration.RunMapping{}, err
	}
	fileName := ""
	if req.FileName != nil {
		fileName = *req.FileName
	}
	if utf8.RuneCountInString(fileName) > maxImportFileName {
		return migration.RunMapping{}, httperr.Validation("file_name", "too_long",
			fmt.Sprintf("A file name is at most %d characters.", maxImportFileName))
	}
	return migration.RunMapping{
		OnDuplicate: onDuplicate, ContextTag: contextTag, OnBrokenLink: onBrokenLink,
		SourceName: sourceName, FileName: fileName,
	}, nil
}

// checkPartColumns proves a part's mapping names columns its file actually
// carries. It runs before anything is staged. A mapping the file contradicts is
// a request fault, like an unknown target, and not a run whose rows drop a field.
func checkPartColumns(ctx context.Context, source *migration.CSVSource, fields map[string]string) error {
	header, err := source.Header(ctx)
	if err != nil {
		return importProblem(err)
	}
	return mappingNamesTheFilesColumns(fields, header)
}

// mappingNamesTheFilesColumns refuses a mapping naming a column this file's
// header does not carry.
//
// The row builder DROPS such a column (CSVSource.rowFrom), so the field it was
// mapped onto imports empty on every row while the preview reports clean
// creates and the commit lands them. `unmapped` cannot disclose it: that list
// names the FILE's columns nothing reads, and a column the file lacks is on
// neither side of it.
//
// Refused rather than reported, beside the unknown-target refusal above: a
// header is one fact about the WHOLE file, so a name it lacks is wrong for
// every row or for none, and there is no row for a row-level issue to name.
// Compared exactly as spelled, because that is how rowFrom looks a column up.
func mappingNamesTheFilesColumns(fields map[string]string, header []string) error {
	carried := make(map[string]bool, len(header))
	for _, name := range header {
		carried[name] = true
	}
	var absent []string
	for column := range fields {
		if !carried[column] {
			absent = append(absent, column)
		}
	}
	if len(absent) == 0 {
		return nil
	}
	// Sorted, because a map's range order would give the same request a
	// different sentence on every call.
	sort.Strings(absent)
	// The absent names lead and the header follows, because this rides to an
	// agent as a field message cut at httperr.MaxFaultText: a wide file's
	// header would otherwise truncate away the very name it is about.
	return httperr.Validation("mapping", "unknown_column",
		fmt.Sprintf("this file has no column called %s. Its header is: %s.",
			strings.Join(quotedNames(absent), ", "), strings.Join(quotedNames(header), ", ")))
}

// quotedNames puts each column name in quotes, so a header with a space in it
// reads as one name in the sentence.
func quotedNames(names []string) []string {
	out := make([]string, 0, len(names))
	for _, name := range names {
		out = append(out, strconv.Quote(name))
	}
	return out
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
	case errors.Is(err, migration.ErrNotUTF8):
		return httperr.Validation("file", "not_utf8", err.Error())
	case errors.Is(err, migration.ErrSourceUnreadable):
		return httperr.Validation("file", "unreadable", err.Error())
	case errors.Is(err, migration.ErrBundleRefused):
		return httperr.Validation("file", "bundle_refused", err.Error())
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
	if m := run.Mapping; m != nil {
		if len(m.Parts) == 0 {
			object := crmcontracts.ImportObject(m.Object)
			out.Object = &object
		}
		if m.FileName != "" {
			out.FileName = &m.FileName
		}
		if m.SourceName != "" {
			out.SourceName = &m.SourceName
		}
		parts := make([]crmcontracts.ImportRunPart, 0, len(m.PartList()))
		for _, p := range m.PartList() {
			parts = append(parts, crmcontracts.ImportRunPart{
				File: p.File, Object: crmcontracts.ImportObject(p.Object), RowCount: rowsOf(run.Report, p.Object),
			})
		}
		out.Parts = &parts
	}
	if run.Error != "" {
		message := run.Error
		out.Error = &message
	}
	return out
}

// rowsOf is how many rows the run's report read for one object; 0 before a
// dry run has read any.
func rowsOf(rep *migration.Report, object string) int {
	if rep == nil {
		return 0
	}
	for _, o := range rep.Objects {
		if o.Object == object {
			return o.MirrorCount
		}
	}
	return 0
}

// importUpload is one uploaded file. Object is empty for a ZIP, whose file
// names say what each part holds.
type importUpload struct {
	Object   string
	FileName string
	Body     []byte
}

// readImportUpload takes the multipart body apart under the deployment's import
// cap and returns the object the rows are, the file's base name and its bytes.
//
// The bytes are read whole rather than streamed to the blobstore, and that is
// deliberate: the same upload must be BOTH profiled and stored, and a stream
// can only be consumed once. The cap is what makes reading it whole safe, and
// MaxBytesReader (not a bare LimitReader) is what turns an over-cap upload into
// a refusal rather than a file silently cut in half.
func readImportUpload(w http.ResponseWriter, r *http.Request, limit int64) (importUpload, error) {
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	// upload:route /v1/imports/sources — the ceiling this parse runs under is granted to that
	// path in compose.uploadCeilings, and TestEveryMultipartParseNamesItsRoute
	// holds the two together.
	//nolint:gosec // G120 wants a bound here, and the bound is the MaxBytesReader above: this argument is only the in-memory/spill threshold, and it is deliberately far below the ceiling so the parse spills rather than holding the upload resident.
	if err := r.ParseMultipartForm(importSpillBytes); err != nil {
		// The cap is DERIVED, never spelled again in prose: this sentence said
		// "10 MB" while the constant beside it decided the real answer, and the
		// two were free to drift the moment either moved.
		return importUpload{}, httperr.MultipartRefusal(err, limit)
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		return importUpload{}, httperr.Validation("file", "missing", "Attach the file to import as `file`.")
	}
	defer func(ctx context.Context) {
		if cerr := file.Close(); cerr != nil {
			slog.WarnContext(ctx, "closing the uploaded import part", "err", cerr)
		}
	}(r.Context())

	body, err := readAllUpload(file)
	if err != nil {
		return importUpload{}, httperr.Validation("file", "unreadable", "The upload could not be read.")
	}
	if len(body) == 0 {
		return importUpload{}, httperr.Validation("file", "empty", "The uploaded file has no content.")
	}
	object := strings.TrimSpace(r.FormValue("object"))
	if _, ok := csvTargets[object]; !ok && !migration.IsZip(body) {
		return importUpload{}, unsupportedObject()
	}
	return importUpload{Object: object, FileName: path.Base(header.Filename), Body: body}, nil
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
