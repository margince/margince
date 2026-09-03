// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// The run_report tool (interfaces.md §2.1, 🟢): reads aggregate rows
// through the compiled report engine. The engine lives above the
// modules (it queries across domain tables), so the composition root
// injects it here as a function — the tool owns the surface contract,
// never the SQL.

import (
	"context"
	"encoding/json"
	"slices"
	"strings"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// ReportRunner executes one named report with the request's plan
// arguments and returns the contract-shaped result JSON.
type ReportRunner func(ctx context.Context, report string, planArgs json.RawMessage) (json.RawMessage, error)

// ReportCatalogEntry is one prebuilt report as a CALLER sees it: its key and
// the three closed vocabularies the engine will accept from them.
//
// The engine owns the catalog (it is where the SQL lives), so the composition
// root hands it over here rather than this package restating it. Every list is
// sorted by its producer, so the rendered description is byte-stable across
// processes — a description that reshuffles per boot reads as a changed tool to
// a client that caches it.
type ReportCatalogEntry struct {
	Report string
	// GroupBy, Filters and Aggregates are the dimension, filter and measure
	// keys this report accepts. Anything else is refused by the engine.
	GroupBy    []string
	Filters    []string
	Aggregates []string
	// Defaults says what the report answers when the plan arguments are
	// omitted, which is the call a caller should make first.
	Defaults string
	// Notes says what a filter MEANS when its name alone does not: which
	// rows `project_id` admits, what unit `days` is in. Empty when every
	// name is self-evident.
	Notes string
}

// RegisterReportTool joins run_report to the surface once the engine
// exists — the same conditional-registration pattern the other
// verb-gated tools follow.
//
// The catalog travels with it because a report key was previously a free-form
// string documented by three examples, and its filter/group_by/aggregate
// vocabularies were documented as "the report's vocabulary" — a phrase naming
// something no tool on this surface yielded. A caller had to guess a key, then
// guess the words that key accepts, and read a refusal for each miss.
func RegisterReportTool(r *Registry, run ReportRunner, catalog []ReportCatalogEntry) {
	r.Register(runReport{run: run, catalog: catalog})
}

type runReport struct {
	run     ReportRunner
	catalog []ReportCatalogEntry
}

func (t runReport) Spec() mcp.ToolSpec {
	return mcp.ToolSpec{
		Name: "run_report", Title: "Run a report", Version: toolVersionV1,
		Description:   runReportCopy.render(),
		RequiredScope: principal.ScopeRead, Tier: mcp.TierAutoExecute,
		OpenAPIOp: "runReport",
		InputSchema: schema(`{"type":"object","required":["report"],"properties":{
			"report":` + reportProperty(t.catalog) + `,
			"filters":{"type":"object","description":"Equality predicates keyed by this report's filter names — {\"owner_id\":\"<uuid>\"}. A key outside the report's list is refused."},
			"group_by":{"type":"array","items":{"type":"string"},"description":"Dimension names from this report's list. Omit for the report's own default grouping."},
			"aggregates":{"type":"array","items":{"type":"object","required":["fn"],"properties":{
				"fn":{"type":"string","enum":["count","sum","avg","min","max"]},
				"field":{"type":"string","description":"A measure name from this report's list. Omit only with fn=count."},
				"as":{"type":"string","description":"Output column name for this aggregate"}},"additionalProperties":false},
				"description":"Omit for the report's own default aggregates."}},
			"additionalProperties":false}`),
		OutputSchema: schemaFor[RunReportResult](),
	}
}

// reportProperty renders the `report` argument, closing it to the catalog's own
// keys so a caller reads the answer instead of guessing it.
//
// An empty catalog omits the enum rather than emitting an empty one: `"enum":[]`
// is a schema no value can satisfy, which advertises a tool that cannot be
// called, and JSON Schema requires an ARRAY there — so there is no honest
// placeholder to write either.
func reportProperty(catalog []ReportCatalogEntry) string {
	described := `"type":"string","description":` + jsonString(describeReportCatalog(catalog))
	if len(catalog) == 0 {
		return "{" + described + "}"
	}
	keys := make([]string, 0, len(catalog))
	for _, entry := range catalog {
		keys = append(keys, jsonString(entry.Report))
	}
	return `{"enum":[` + strings.Join(keys, ",") + `],` + described + `}`
}

// describeReportCatalog says what each report ANSWERS, and names where the
// three plan vocabularies are rather than reciting them.
//
// It used to recite everything: every report's group_by, filters, aggregates,
// default and note, in this one property. That was 3.4KB — 6% of the served
// catalog in a single tool, held by every client for a whole session and re-sent
// by every Surface-B run on every step.
//
// WHERE THE LINE FALLS is the whole of this function, and it was MEASURED
// rather than reasoned. The first pass deferred the DEFAULTS along with the
// vocabularies, leaving an enum of bare report keys. The certification lane
// answered that immediately and twice: on the goal "how much open pipeline do we
// have in each stage", every run reached for the vocabulary door instead of the
// report — 2/3 to 0/3 — and it was RIGHT to. `deals-by-stage` answers exactly
// that goal with no plan at all, and a bare key does not say so. The recital's
// `default:` line was carrying the selection signal; nothing else on this
// surface does.
//
// So the split is by QUESTION, not by size:
//
//   - Which report answers my goal, and do I need a plan at all? Answered here,
//     by each key and its default, at zero round trips. It is the cheap half —
//     ~180 tokens for nine reports — and it is what a caller needs FIRST.
//   - What may a plan SAY? The three closed name lists, which only a caller who
//     is narrowing needs at all. That is the 3.4KB, and it is the document's.
//
// The sentence NAMES the document and does not order a read
// (TestNoToolOrdersTheModelToReadADocument), and it says the document is NOT a
// prerequisite: naming one beside an argument reads as an order even with no
// imperative in it, so the default call is stated first and says outright that
// it needs nothing read.
func describeReportCatalog(catalog []ReportCatalogEntry) string {
	if len(catalog) == 0 {
		return "No prebuilt report is available on this installation."
	}
	var b strings.Builder
	b.WriteString("The prebuilt report to run. Send `report` ALONE for the default answer listed ")
	b.WriteString("below — that call takes no other argument and needs nothing read first. ")
	writeReportDefaults(&b, catalog)
	b.WriteString("To narrow one instead, its `group_by`, `filters` and `aggregates` accept ONLY ")
	b.WriteString("that report's own names, published at ")
	b.WriteString(ReportVocabularyURI)
	b.WriteString(" and answered by describe_report_vocabulary; a name outside them is refused by ")
	b.WriteString("name, with that argument's accepted list.")
	writePipelineSource(&b, catalog)
	return b.String()
}

// writePipelineSource closes the obligation the defaults above open: several
// reports group by `pipeline_id` and `stage_id`, and naming an id a caller
// cannot obtain is a correct refusal that dead-ends.
//
// It sits HERE as well as in the document because the obligation follows the
// NAMES: TestEveryToolNeedingAPipelineOrStageIDPointsAtListPipelines reads the
// input schema, and restoring the defaults put those ids back into it. When the
// first pass deferred the defaults, the ids left the schema and this sentence
// went with them — correctly. They are back, so it is.
//
// The predicate and the sentence are SHARED with the document rather than
// re-typed there: they were two copies of one obligation and had already
// drifted by a word.
func writePipelineSource(b *strings.Builder, catalog []ReportCatalogEntry) {
	if catalogNamesAPipelineID(catalog) {
		b.WriteString(" ")
		b.WriteString(pipelineIDProvenance)
	}
}

// pipelineIDProvenance is the one sentence saying where those ids come from,
// read by run_report's description and by the published document.
const pipelineIDProvenance = "A `pipeline_id` or `stage_id` used in a plan comes from list_pipelines."

// catalogNamesAPipelineID reports whether any rendered vocabulary names an id a
// caller has to obtain elsewhere. Keyed on what is actually rendered, so a
// catalog without those keys carries no advice about them.
func catalogNamesAPipelineID(catalog []ReportCatalogEntry) bool {
	for _, entry := range catalog {
		for _, names := range [][]string{entry.GroupBy, entry.Filters, entry.Aggregates} {
			if slices.Contains(names, "pipeline_id") || slices.Contains(names, "stage_id") {
				return true
			}
		}
	}
	return false
}

// RenderedReportKeyGuidance is what run_report's `report` argument says about
// the catalog, for the composition to hold against the engine's own specs.
//
// Exported for ONE reader: the gate that every enum key is described by its
// default answer. That obligation belongs to the composition, which owns the
// prebuilt specs, and it cannot be checked without the rendered text this
// package produces.
func RenderedReportKeyGuidance(catalog []ReportCatalogEntry) string {
	return describeReportCatalog(catalog)
}

// writeReportDefaults renders what each report answers with no plan — the one
// thing a caller cannot infer from a report's key.
//
// Keyed on the entries that HAVE a default, so a report with none stays silent
// rather than rendering a key with an empty clause after it, which would read as
// a report that answers nothing.
func writeReportDefaults(b *strings.Builder, catalog []ReportCatalogEntry) {
	for _, entry := range catalog {
		if entry.Defaults == "" {
			continue
		}
		b.WriteString(entry.Report)
		b.WriteString(": ")
		b.WriteString(entry.Defaults)
		b.WriteString(". ")
	}
}

func (t runReport) Handle(ctx context.Context, in json.RawMessage) (json.RawMessage, error) {
	var args struct {
		Report string          `json:"report"`
		Rest   json.RawMessage `json:"-"`
	}
	if err := decodeReportArgs(in, &args.Report, &args.Rest); err != nil {
		return nil, err
	}
	noteDerivedContent(ctx)
	return t.run(ctx, args.Report, args.Rest)
}

// decodeReportArgs pops the report key and forwards the remaining plan
// arguments verbatim — the engine validates the vocabulary.
func decodeReportArgs(in json.RawMessage, report *string, rest *json.RawMessage) error {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(in, &m); err != nil {
		return &BadArgsError{Cause: err}
	}
	raw, ok := m["report"]
	if !ok || json.Unmarshal(raw, report) != nil || *report == "" {
		return &BadArgsError{Cause: errMissingReport}
	}
	delete(m, "report")
	remaining, err := json.Marshal(m)
	if err != nil {
		return err
	}
	*rest = remaining
	return nil
}

var errMissingReport = jsonError("a report key is required")

type jsonError string

func (e jsonError) Error() string { return string(e) }
