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

// describeReportCatalog names where the three plan vocabularies are, rather
// than reciting them.
//
// It used to recite them: every report's group_by, filters, aggregates, default
// and note, rendered into this one property. That was 3.4KB — 6% of the served
// catalog in a single tool, held by every client for a whole session and
// re-sent by every Surface-B run on every step, to answer a question one call
// asks once. The recital moved to margince://schema/reports, with
// describe_report_vocabulary as the door for a caller that reads no resources.
//
// What is NOT deferred is the enum: `report` stays closed to the catalog's own
// keys, because that is what decides whether a call is well-formed and it
// answers the first of a caller's two questions at zero round trips.
//
// The sentence NAMES the document and does not order a read. A binding told to
// "read this first" obeyed on goals with no report in them and lost first-step
// accuracy; TestNoToolOrdersTheModelToReadADocument holds the phrasing.
//
// AND IT SAYS THE DOCUMENT IS NOT A PREREQUISITE, which is a second obligation
// the first pass missed. Naming a document beside an argument READS as an order
// even with no imperative in it: the certification lane's first run of this
// shape had a binding open describe_report_vocabulary on all three runs of a
// goal whose answer is one default report call — spending its whole turn on the
// vocabulary and answering nothing. So the plain call is stated FIRST and
// explicitly needs nothing, and the vocabulary is scoped to narrowing.
func describeReportCatalog(catalog []ReportCatalogEntry) string {
	if len(catalog) == 0 {
		return "No prebuilt report is available on this installation."
	}
	var b strings.Builder
	b.WriteString("The prebuilt report to run. Send `report` ALONE for its default answer — that ")
	b.WriteString("call needs no other argument and nothing read first. ")
	b.WriteString("To narrow it instead, each report's `group_by`, `filters` and `aggregates` accept ")
	b.WriteString("ONLY that report's own names, published at ")
	b.WriteString(ReportVocabularyURI)
	b.WriteString(" and answered by describe_report_vocabulary; a name outside them is refused by ")
	b.WriteString("name, with that argument's accepted list.")
	return b.String()
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
