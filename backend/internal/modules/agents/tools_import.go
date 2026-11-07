// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// The import verbs: bringing a spreadsheet of records into the estate, which
// the tool surface could not begin.
//
// Every import operation was human-only, so an assistant handed a CSV could
// describe what to do with it and do none of it. The whole migrate-in flow —
// the one a customer runs once, on their first day, with the most data at
// stake — was reachable only by a contact clicking through four screens.
//
// THE FILE ARRIVES AS TEXT, not as an upload. uploadImportSource is multipart
// and stays human-only, because an assistant holding a spreadsheet's contents
// cannot perform a file upload and asking it to construct a multipart body
// would be a worse door than none. `preview_import` takes the CSV as a string
// instead: the assistant already has the characters, and this is the shape it
// can actually produce.
//
// THE DRY RUN IS NOT OPTIONAL AND CANNOT BE SKIPPED. preview_import writes no
// domain rows by construction (AC-M5) — it validates the mapping against the
// live estate and produces a report. commit_import is confirm-first and takes
// a run id, so the thing a human approves is the run whose report they read.
// There is no verb that imports without producing a report first, and adding
// one would defeat the only review this flow has.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/baselanguage"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// Imports is the seam onto the migration module's CSV paths. The composition
// root implements it over the same handlers the REST transport uses, so both
// doors run one dry run and one commit.
type Imports interface {
	// ProfileSource stores CSV text and reads back what its columns contain,
	// with a proposed mapping. The agent-shaped half of uploadImportSource.
	ProfileSource(ctx context.Context, object, csv string) (crmcontracts.ImportSourceProfile, error)
	// DiscardSource removes a stored source no run will ever reference.
	//
	// The tool path stores the file BEFORE it can know whether the call will
	// succeed, so a refusal after that point leaves an orphan blob — storage a
	// caller can spend by repeating a call that fails. importSeam.ProfileSource
	// hoists the authorization check for exactly that reason; this is the same
	// concern one step later, where the refusal is about the file's own shape
	// rather than about who is asking.
	DiscardSource(ctx context.Context, ref string) error
	// StageRun validates a mapping against the estate and parks the run for a
	// human. Writes no domain rows.
	StageRun(ctx context.Context, req crmcontracts.CreateImportRunRequest) (crmcontracts.ImportRun, error)
	// ReadRun answers one run's lifecycle state.
	ReadRun(ctx context.Context, id ids.UUID) (crmcontracts.ImportRun, error)
	// ReadReport answers the run and what it will do, or did. The report is
	// nil until validation has produced one.
	ReadReport(ctx context.Context, id ids.UUID) (crmcontracts.ImportRun, *crmcontracts.ImportRunReport, error)
	// Commit approves a validated run and starts the write.
	Commit(ctx context.Context, id ids.UUID) (crmcontracts.ImportRun, error)
	// DescribeFormat answers the import format in the installation's language.
	DescribeFormat(ctx context.Context, lang string) (crmcontracts.ImportFormat, error)
	// ProfileFiles stores several CSV files as one bundle and profiles each,
	// as an uploaded ZIP is.
	ProfileFiles(ctx context.Context, files []ImportFile) (crmcontracts.ImportSourceProfile, error)
}

// ImportFile is one CSV of a several-file import and the type its rows are.
type ImportFile struct {
	Object string
	CSV    string
}

// RegisterImportTools joins the import verbs to the surface.
//
// Unconditionally, unlike the tag and whoami registrars. The contract declares
// these verbs, and TestEveryDeclaredToolVerbIsRegistered fails a registry that
// skips one. A deployment with no object store still serves them, and refuses
// the ones that need the source file, naming what is missing.
func RegisterImportTools(r *Registry, imports Imports) {
	r.Register(previewImport{imports: imports, wait: r.importWait})
	r.Register(readImportReport{imports: imports})
	r.Register(describeImportFormat{imports: imports})
	r.Register(commitImport{imports: imports, language: r.language})
	r.Register(readImport{})
}

// importObjectEnum is what a file's rows may be, held to the contract by
// TestEveryToolEnumMatchesTheContractItMirrors.
//
// `lead` and `contact` are BOTH here and the caller picks per run: a
// machine-sourced list lands as leads for a human to promote, a file the
// business already knows lands as contacts. Neither skips the identity ladder.
var importObjectEnum = []string{importObjectCompany, importObjectLead, importObjectContact, importObjectDeal, importObjectActivity}

// importObjectEnumJSON is importObjectEnum as the JSON array a tool schema's enum takes.
func importObjectEnumJSON() string { return `["` + strings.Join(importObjectEnum, `","`) + `"]` }

// The things a file's rows may be, spelled once. They mirror the contract's
// ImportObject enum, and the tool's schema is built from them so the two
// cannot drift.
const (
	importObjectCompany  = "company"
	importObjectLead     = "lead"
	importObjectContact  = "contact"
	importObjectDeal     = "deal"
	importObjectActivity = "activity"
)

// maxImportCSVBytes caps the pasted file.
//
// BYTES, not characters — len() on a Go string counts bytes, and calling this
// a character count would be a lie that matters: a UTF-8 file of German or
// Vietnamese company names is refused sooner than the number suggests. The
// refusal says bytes.
//
// The bound is well under the REST upload limit (uploads.csv_import_mb, 10 MB
// by default) and under the MCP transport's own 8 MiB body: text travelling
// inside a JSON tool argument rides the same request as the rest of the
// conversation, and a file large enough to matter belongs on the upload door
// with a contact at it.
const maxImportCSVBytes = 1_000_000

// importConnectorCSV is the connector a pasted file is: the same one the
// upload door uses, because it IS the same path — only the way the bytes
// arrived differs.
const importConnectorCSV = "csv"

// The two duplicate policies, spelled here so the tool schema and the contract
// enum cannot drift apart.
const (
	importOnDuplicateCreate = string(crmcontracts.ImportOnDuplicateCreate)
	importOnDuplicateSkip   = string(crmcontracts.ImportOnDuplicateSkip)
)

// The two broken-link policies, spelled here for the same reason.
const (
	importOnBrokenLinkSkipRow  = string(crmcontracts.ImportOnBrokenLinkSkipRow)
	importOnBrokenLinkDropLink = string(crmcontracts.ImportOnBrokenLinkDropLink)
)

// refuseUnimportableObject holds `object` to the vocabulary, naming the whole
// of it rather than saying the value is invalid.
func refuseUnimportableObject(object string) error {
	if slices.Contains(importObjectEnum, object) {
		return nil
	}
	// The caller's word in Cause, which is bounded and escaped, and the set in
	// Guidance, which is ours. Both in Cause meant the vocabulary shared a
	// budget sized for an echo, and the refusal never said which value was
	// wrong — only what the right ones were.
	return &BadArgsError{
		Cause:    fmt.Errorf("`object` %q is not one this tool imports", object),
		Field:    "object",
		Guidance: "it is one of: " + strings.Join(importObjectEnum, ", "),
	}
}

type readImportReport struct{ imports Imports }

func (t readImportReport) Spec() mcp.ToolSpec {
	return mcp.ToolSpec{
		Name: "read_import_report", Title: "Read an import run and its report", Version: toolVersionV1,
		Description:   readImportReportCopy.render(),
		Instead:       readImportReportCopy.Instead,
		RequiredScope: principal.ScopeRead, Tier: mcp.TierAutoExecute, Unlisted: true,
		OpenAPIOp:    "getImportRunReport",
		InputSchema:  schema(`{"type":"object","required":["run_id"],"properties":{"run_id":{"type":"string","format":"uuid"}},"additionalProperties":false}`),
		OutputSchema: schemaFor[ImportReportResult](),
	}
}

func (t readImportReport) Handle(ctx context.Context, in json.RawMessage) (json.RawMessage, error) {
	id, err := importRunArg(in)
	if err != nil {
		return nil, err
	}
	run, report, err := t.imports.ReadReport(ctx, id)
	if err != nil {
		return nil, err
	}
	out := ImportReportResult{
		RunID: run.Id.String(), State: string(run.Status), Checkpoint: run.Checkpoint, Report: report,
	}
	if run.Error != nil {
		out.Error = *run.Error
	}
	if run.Parts != nil {
		out.Parts = *run.Parts
	}
	return json.Marshal(out)
}

type commitImport struct {
	imports  Imports
	language baselanguage.Resolver
}

func (t commitImport) Spec() mcp.ToolSpec {
	return mcp.ToolSpec{
		Name: "commit_import", Title: "Commit an import", Version: toolVersionV1,
		AskUserFirst:  true,
		Description:   commitImportCopy.render(),
		Instead:       commitImportCopy.Instead,
		RequiredScope: principal.ScopeWrite, Tier: mcp.TierAutoExecute,
		OpenAPIOp: "approveImportRun",
		InputSchema: schema(`{"type":"object","required":["run_id"],"properties":{
			"run_id":{"type":"string","format":"uuid"},
			"approval_id":{"type":"string","format":"uuid","description":"Set on approved retry"}},
			"additionalProperties":false}`),
		OutputSchema: schemaFor[ImportRunResult](),
	}
}

// StageInfo describes the approval a commit asks for, through the SAME
// command the REST door builds — so the sentence a human decides on is
// identical whichever door staged it.
func (t commitImport) StageInfo(ctx context.Context, in json.RawMessage) (StageInfo, error) {
	id, err := importRunArg(in)
	if err != nil {
		return StageInfo{}, err
	}
	return StageSubject(ctx, NewImportCall(t.imports, t.language, ImportCommand{
		Verb: ImportVerbCommit, RunID: id,
	}))
}

func (t commitImport) Handle(ctx context.Context, in json.RawMessage) (json.RawMessage, error) {
	id, err := importRunArg(in)
	if err != nil {
		return nil, err
	}
	// The seam is checked here, not only where an approval used to be staged.
	// A deployment with no object store still SERVES this verb — the contract
	// declares it, so the registry advertises it — and a nil seam reached by a
	// direct call panics on the read below rather than naming what is missing.
	if t.imports == nil {
		return nil, fmt.Errorf(
			"no import seam is wired, so run %s cannot be committed here", id)
	}
	// Checked again on the approved retry. StageInfo's check is the courtesy
	// that avoids spending an approval; this one is the rule, because Handle
	// is what a granted approval re-enters and the run's state can have moved
	// while the contact was deciding.
	run, err := t.imports.ReadRun(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := refuseUncommittableRun(run); err != nil {
		return nil, err
	}
	committed, err := t.imports.Commit(ctx, id)
	if err != nil {
		return nil, err
	}
	return json.Marshal(importRunResult(committed))
}

// importRunRecordType is the word the contract's admission policy uses for an
// import run, so a staged approval is row-scoped to the run it names.
const importRunRecordType = "import_run"

// awaitingApproval is the one state a commit may start from.
const awaitingApproval = "awaiting_approval"

// refuseUncommittableRun holds a commit to a run that has been dry-run, or to a
// failed commit, which resumes at its checkpoint. A run reaches
// awaiting_approval only by producing a report, and only an approved run was
// ever committed, so the dry run stays mandatory. The refusal names the state:
// "already running" and "already finished" are what a caller must tell apart.
func refuseUncommittableRun(run crmcontracts.ImportRun) error {
	status := string(run.Status)
	if status == awaitingApproval || (status == failedCommit && run.ApprovedAt != nil) {
		return nil
	}
	return &BadArgsError{Cause: fmt.Errorf(
		"import run %s is %s, and a commit starts only from %s or resumes a failed commit — "+
			"an import commits what a dry run reported, so there is always a report first",
		run.Id, status, awaitingApproval)}
}

// failedCommit is the state of a commit that stopped part-way.
const failedCommit = "failed"

// importRunArg reads the one argument every id-bearing import verb takes.
func importRunArg(in json.RawMessage) (ids.UUID, error) {
	var args struct {
		RunID string `json:"run_id"`
	}
	if err := decodeArgs(in, &args); err != nil {
		return ids.UUID{}, err
	}
	id, err := ids.Parse(args.RunID)
	if err != nil {
		return ids.UUID{}, &BadArgsError{Cause: errors.New(
			"`run_id` must be the uuid preview_import answered with")}
	}
	return id, nil
}

// importObjectName is the object a single-CSV run or profile names, and empty
// for a ZIP, which names one per part.
func importObjectName(object *crmcontracts.ImportObject) string {
	if object == nil {
		return ""
	}
	return string(*object)
}

// importRunResult reshapes one run for the wire.
func importRunResult(run crmcontracts.ImportRun) ImportRunResult {
	out := ImportRunResult{
		RunID:      run.Id.String(),
		Object:     importObjectName(run.Object),
		State:      string(run.Status),
		Checkpoint: run.Checkpoint,
	}
	if run.Error != nil {
		out.Error = *run.Error
	}
	return out
}
