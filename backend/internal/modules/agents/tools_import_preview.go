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
	"strconv"
	"strings"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

type previewImport struct{ imports Imports }

func (t previewImport) Spec() mcp.ToolSpec {
	return mcp.ToolSpec{
		Name: "preview_import", Title: "Preview an import", Version: toolVersionV1,
		Description:   previewImportCopy.render(),
		RequiredScope: principal.ScopeWrite, Tier: mcp.TierAutoExecute,
		OpenAPIOp: "createImportRun",
		InputSchema: schema(`{"type":"object","required":["object","csv"],"properties":{
			"object":{"type":"string","enum":["` + importObjectOrganization + `","` + importObjectLead + `","` + importObjectPerson + `"]},
			"csv":{"type":"string","description":"The file's contents, header row first."},
			"mapping":{"type":"object","additionalProperties":{"type":"string"},
			  "description":"Source column name → field name. Omit to accept the proposal this call would make, which it will only make if it can place EVERY column — a file whose headers are spelled the way a human would (\"Company\", \"City\") matches no field by name and is refused with the list, so send a mapping for those. Map a column to \"id\" to name the company a row corrects: that row updates it instead of creating one. A row whose \"id\" is empty is a new company, so one file may both correct and add. On a PERSON run, map the company column to \"organization_name\" to link each person to their employer: the company must already be in the CRM, so import companies first, and a name matching none or matching two links nothing while the person still lands."},
			"on_duplicate":{"type":"string","enum":["` + importOnDuplicateCreate + `","` + importOnDuplicateSkip + `"],
			  "description":"A record already here: create (default) lands a second and files the pair for review; skip leaves the incumbent. For people an address already held is refused either way — an email is a real key, a company name is not."}},
			"additionalProperties":false}`),
		OutputSchema: schemaFor[ImportPreviewResult](),
	}
}

func (t previewImport) Handle(ctx context.Context, in json.RawMessage) (json.RawMessage, error) {
	var args struct {
		Object      string            `json:"object"`
		CSV         string            `json:"csv"`
		Mapping     map[string]string `json:"mapping"`
		OnDuplicate string            `json:"on_duplicate"`
	}
	if err := decodeArgs(in, &args); err != nil {
		return nil, err
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
	// each column became and a person reads it before anything commits.
	mapping := make(map[string]string, len(profile.SuggestedMapping))
	for column, field := range profile.SuggestedMapping {
		mapping[column] = field
	}
	for column, field := range args.Mapping {
		mapping[column] = field
	}
	if len(args.Mapping) == 0 {
		if err := proposalCoversTheFile(profile, mapping); err != nil {
			return nil, discarding(ctx, t.imports, profile.SourceRef, err)
		}
	}

	req := crmcontracts.CreateImportRunRequest{
		Connector: crmcontracts.CreateImportRunRequestConnector(importConnectorCSV),
		Object:    profile.Object,
		SourceRef: profile.SourceRef,
		Mapping:   mapping,
	}
	if args.OnDuplicate != "" {
		policy := crmcontracts.ImportOnDuplicate(args.OnDuplicate)
		req.OnDuplicate = &policy
	}
	run, err := t.imports.StageRun(ctx, req)
	if err != nil {
		// NOT discarded. Staging persists its run before it validates, and a
		// run that failed validation is resumable from its SourceRef — so
		// deleting the file here would turn a run somebody can fix into one
		// nobody can. The orphan on this path is the pre-existing cost of a
		// tool call that stores before it can know, and it is a different
		// change from this one.
		return nil, err
	}
	return json.Marshal(ImportPreviewResult{
		Run:      importRunResult(run),
		Mapping:  mapping,
		Columns:  columnNames(profile),
		Unmapped: unmappedColumns(profile, mapping),
	})
}

// discarding removes the stored source behind a call that is about to fail, and
// returns the failure unchanged.
//
// Only for a refusal BEFORE anything is staged. Past that point a run exists
// that references the source and can be resumed from it, and deleting the file
// would turn a run somebody can fix into one nobody can.
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

// discardTimeout bounds the cleanup. It outlives the request on purpose, and a
// store that has not answered in this long is not going to.
const discardTimeout = 10 * time.Second

// proposalCoversTheFile refuses a proposal that places some of the file's
// columns and not others, when the caller sent no mapping of their own.
//
// The proposal matches NAMES, and nothing more (migration.SuggestMapping); the
// timidity is right for the screen, which draws an unplaced column as a blank
// the person fills. A tool caller has no blanks. Handed a proposal that maps
// `id` and drops `Company`, `City`, `Country` and `Band`, they get something
// plausible that validates clean and commits an update with no changed fields
// — an import that reports success and writes nothing. A partial answer that
// looks whole is worse than no answer, so this is the refusal that says which
// columns it could not place and what the object can receive.
//
// Only when the caller named NO column — whether the member was omitted or sent
// as `{}`, which are the same thing to ask about: a mapping that names nothing
// is not a choice about the columns, it is the absence of one, and what would
// be used either way is the proposal. A caller who named even one column HAS
// made a choice about the rest, and the result's `unmapped` list reports it.
func proposalCoversTheFile(profile crmcontracts.ImportSourceProfile, mapping map[string]string) error {
	unplaced := unmappedColumns(profile, mapping)
	if len(unplaced) == 0 {
		return nil
	}
	// Both LISTS ride in Guidance, which is not bounded. The cause is echoed
	// through echoSafe's 200 bytes, and the fixed prose alone is most of that —
	// so a list put there is cut off partway through, which on a wide file
	// means the refusal stops naming the very columns it is about. What is left
	// in the cause is the sentence that is the same length whatever the file
	// holds.
	return &BadArgsError{
		Cause: fmt.Errorf(
			"the proposal places %d of this file's %d columns, so importing it would leave the "+
				"rest of every row behind and report success",
			len(mapping), len(mapping)+len(unplaced)),
		Guidance: fmt.Sprintf(
			"it could not place %s, because they match no %s field by name. Send a mapping "+
				"that says what each column is. A %s takes: %s",
			strings.Join(quoted(unplaced), ", "), profile.Object,
			profile.Object, strings.Join(profile.Targets, ", ")),
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
