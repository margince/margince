// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// Bringing a file in: the half of the import verbs that takes a spreadsheet's
// bytes and stages a run somebody can approve.
//
// Separate from the verbs that READ a staged run and commit it, because this
// half is where the file is stored before anything can know whether the call
// will succeed — and every refusal past that point has to decide what becomes
// of it.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"strconv"
	"strings"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// previewImport stages a run's test and waits for its report.
type previewImport struct {
	imports Imports
	wait    importWait
}

func (t previewImport) Spec() mcp.ToolSpec {
	return mcp.ToolSpec{
		Name: "preview_import", Title: "Preview an import", Version: toolVersionV1,
		Description:   previewImportCopy.render(),
		Instead:       previewImportCopy.Instead,
		RequiredScope: principal.ScopeWrite, Tier: mcp.TierAutoExecute,
		OpenAPIOp: "createImportRun",
		InputSchema: schema(`{"type":"object","properties":{
			"object":{"type":"string","enum":` + importObjectEnumJSON() + `},
			"csv":{"type":"string","description":"The file's contents, header row first."},
			"mapping":{"type":"object","additionalProperties":{"type":"string"},
			  "description":"Source column name → field name. Omit to accept the proposal this call would make, which it will only make if it can place EVERY column — a file whose headers are spelled the way a human would (\"Company\", \"City\") matches no field by name and is refused with the list, so send a mapping for those. Map a column to \"id\" to name the company a row corrects: that row updates it instead of creating one. A row whose \"id\" is empty is a new company, so one file may both correct and add. On a CONTACT run, map the company column to \"company_name\" to link each contact to their employer: the company must already be in the CRM, so import companies first, and a name matching none or matching two links nothing while the contact still lands."},
			"files":{"type":"array","minItems":1,"maxItems":` + strconv.Itoa(len(importObjectEnum)) + `,
			  "description":"Several files as one run, in place of object, csv and mapping.",
			  "items":{"type":"object","required":["name","object","csv"],"properties":{
				"name":{"type":"string"},
				"object":{"type":"string","enum":` + importObjectEnumJSON() + `},
				"csv":{"type":"string","description":"The file's contents, header row first."},
				"mapping":{"type":"object","additionalProperties":{"type":"string"},"description":"As mapping, for this file."}},
				"additionalProperties":false}},
			"source_name":{"type":"string","description":"The system the files came from. A later import under the same name updates the rows it landed."},
			"on_broken_link":{"type":"string","enum":["` + importOnBrokenLinkSkipRow + `","` + importOnBrokenLinkDropLink + `"],
			  "description":"A link naming no record: skip_row (default) leaves the row out, drop_link lands it unlinked."},
			"on_duplicate":{"type":"string","enum":["` + importOnDuplicateCreate + `","` + importOnDuplicateSkip + `"],
			  "description":"A record already here: create (default) lands a second and files the pair for review; skip leaves the incumbent. For contacts an address already held is refused either way — an email is a real key, a company name is not."}},
			"additionalProperties":false}`),
		UnkeyedArguments: map[string]string{
			"$.mapping":             "the mapping is keyed by the uploaded file's own column headers",
			"$.files.items.mapping": "each file's mapping is keyed by that file's own column headers",
		},
		OutputSchema: schemaFor[ImportPreviewResult](),
	}
}

func (t previewImport) Handle(ctx context.Context, in json.RawMessage) (json.RawMessage, error) {
	var args previewArgs
	if err := decodeArgs(in, &args); err != nil {
		return nil, err
	}
	if len(args.Files) > 0 {
		if args.Object != "" || args.CSV != "" || args.Mapping != nil {
			return nil, &BadArgsError{Cause: errors.New(
				"`files` replaces `object`, `csv` and `mapping`; send one shape or the other")}
		}
		return t.previewFiles(ctx, args)
	}
	if err := refuseUnimportableObject(args.Object); err != nil {
		return nil, err
	}
	if strings.TrimSpace(args.CSV) == "" {
		return nil, &BadArgsError{Cause: errors.New(
			"`csv` is empty; send the file's contents with its header row first")}
	}
	if len(args.CSV) > maxImportCSVBytes {
		return nil, &BadArgsError{Cause: fmt.Errorf(
			"this file is %d bytes and one pasted import takes at most %d; "+
				"upload it in the web app instead, or split it",
			len(args.CSV), maxImportCSVBytes)}
	}

	profile, err := t.imports.ProfileSource(ctx, args.Object, args.CSV)
	if err != nil {
		return nil, err
	}
	// The caller's mapping wins where it names a column, and the proposal
	// fills the rest. A caller that sends none gets the proposal whole —
	// which is honest rather than convenient, because the report says what
	// each column became and a reader reads it before anything commits.
	mapping := make(map[string]string, len(profile.SuggestedMapping))
	maps.Copy(mapping, profile.SuggestedMapping)
	maps.Copy(mapping, args.Mapping)
	if len(args.Mapping) == 0 {
		if err := proposalCoversTheFile(profile, mapping); err != nil {
			return nil, discarding(ctx, t.imports, profile.SourceRef, err)
		}
	}

	req := crmcontracts.CreateImportRunRequest{
		Connector: crmcontracts.CreateImportRunRequestConnector(importConnectorCSV),
		Object:    profile.Object,
		SourceRef: profile.SourceRef,
		Mapping:   &mapping,
	}
	args.applyRunOptions(&req)
	run, err := t.imports.StageRun(ctx, req)
	if err != nil {
		return nil, stagingRefused(ctx, t.imports, profile.SourceRef, err)
	}
	return t.answered(ctx, run, ImportPreviewResult{
		Mapping:  mapping,
		Columns:  columnNames(profile),
		Unmapped: unmappedColumns(profile, mapping),
	})
}

// discarding cancels the draft of a call about to fail before its test, which
// releases the stored source. It returns the failure unchanged.
//
// The refusal is what the caller needs to read, so a failure to clean up cannot
// replace it: an orphan blob is a cost, and the wrong error is a wrong answer.
func discarding(ctx context.Context, imports Imports, ref string, cause error) error {
	if ref == "" {
		return cause
	}
	// Detached from the request's cancellation, and bounded on its own. The
	// caller hanging up between the store and the refusal is exactly when the
	// file is most certainly unwanted, and a cleanup that took the cancelled
	// context with it would leave the orphan precisely then — a caller could
	// spend storage by cancelling.
	cleanup, done := context.WithTimeout(context.WithoutCancel(ctx), discardTimeout)
	defer done()
	//craft:ignore swallowed-errors the refusal below is the answer; a store that would not delete is a cost to carry, not a reason to report something else
	_ = imports.DiscardSource(cleanup, ref)
	return cause
}

// busyRunCopy is what a busy refusal tells the caller to do with the run it
// names, by the refusal's code.
var busyRunCopy = map[string]string{
	httperr.SourceBusyCode: "import run %s of this source_name is still writing. read_import that run " +
		"until its state is complete or failed, then preview again",
	httperr.AgentBusyCode: "import run %s is this agent's test of another file, still running. read_import that run " +
		"(view report) until its state is no longer validating (awaiting_approval, pending or cancelled), " +
		"then preview again",
}

// stagingRefused answers a refused StageRun. A refusal for this agent's other
// test discards the draft. The caller is told to preview again, so a looping
// caller would leave one upload per call. Any other refused draft is kept.
// createImportRun on its source_ref tests it again, and the sweep cancels one
// nobody returns to.
func stagingRefused(ctx context.Context, imports Imports, ref string, err error) error {
	if busy, ok := errors.AsType[*httperr.DetailedError](err); ok && busy.Code == httperr.AgentBusyCode {
		err = discarding(ctx, imports, ref, err)
	}
	return namingTheBusyRun(err)
}

// namingTheBusyRun points a busy refusal at the run it names, when the caller
// may read that run.
func namingTheBusyRun(err error) error {
	busy, ok := errors.AsType[*httperr.DetailedError](err)
	if !ok {
		return err
	}
	detail, known := busyRunCopy[busy.Code]
	runID, named := busy.Details["run_id"].(string)
	if !known || !named {
		return err
	}
	return &httperr.DetailedError{
		Status: busy.Status, Code: busy.Code, Details: busy.Details, Detail: fmt.Sprintf(detail, runID),
	}
}

// discardTimeout bounds the cleanup. It outlives the request on purpose, and a
// store that has not answered in this long is not going to.
const discardTimeout = 10 * time.Second

// proposalCoversTheFile refuses a proposal that places some of the file's
// columns and not others, when the caller sent no mapping of their own.
//
// The proposal matches names only, and a tool caller has no blanks to fill.
// A partial proposal could commit an import that writes nothing. A caller who
// named one column chose the rest, and `unmapped` reports it.
func proposalCoversTheFile(profile crmcontracts.ImportSourceProfile, mapping map[string]string) error {
	unplaced := unmappedColumns(profile, mapping)
	if len(unplaced) == 0 {
		return nil
	}
	// Both lists ride in Guidance, which is not bounded. The cause is echoed
	// through echoSafe's 200 bytes, so a list there would be cut off on a wide
	// file.
	object := importObjectName(profile.Object)
	takes := "read_import with view format lists the columns a " + object + " takes"
	if len(profile.Targets) > 0 {
		takes = fmt.Sprintf("A %s takes: %s", object, strings.Join(profile.Targets, ", "))
	}
	return &BadArgsError{
		Cause: fmt.Errorf(
			"the proposal places %d of this file's %d columns, so importing it would leave the "+
				"rest of every row behind and report success",
			len(mapping), len(mapping)+len(unplaced)),
		Guidance: fmt.Sprintf(
			"it could not place %s, because they match no %s field by name. Send a mapping "+
				"that says what each column is. %s",
			strings.Join(quoted(unplaced), ", "), object, takes),
	}
}

// quoted puts each name in quotes so a header with a space in it reads as one
// name in the sentence.
func quoted(names []string) []string {
	out := make([]string, 0, len(names))
	for _, name := range names {
		out = append(out, strconv.Quote(name))
	}
	return out
}

// columnNames lists what the file actually contains, so a caller that guessed
// a mapping can see the header it guessed against.
func columnNames(p crmcontracts.ImportSourceProfile) []string {
	out := make([]string, 0, len(p.Columns))
	for _, c := range p.Columns {
		out = append(out, c.Header)
	}
	return out
}

// unmappedColumns names the columns nothing will read.
//
// It is reported rather than refused: a file usually carries columns this
// product has no field for, and dropping them is the normal outcome. What is
// not acceptable is dropping them SILENTLY — a caller who mistyped a field
// name would otherwise see a clean report and a column quietly missing.
func unmappedColumns(p crmcontracts.ImportSourceProfile, mapping map[string]string) []string {
	var out []string
	for _, c := range p.Columns {
		if field, ok := mapping[c.Header]; !ok || field == "" {
			out = append(out, c.Header)
		}
	}
	return out
}
