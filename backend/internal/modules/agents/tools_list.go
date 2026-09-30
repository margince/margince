// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// The list_records tool (BYO-TOOL-21, 🟢): enumeration, which this surface had
// no verb for.
//
// search_records finds a record a caller can already half-name. "Every deal in
// this pipeline" is a different question, and the only way to ask it was to
// invent a search term and hope — which returns a subset nobody can size, in the
// shape of a complete answer.
//
// THE FILTER VOCABULARY IS NOT AUTHORED HERE. It is the intersection of two
// things this package is not allowed to decide: what the contract's own list
// operation declares (A139 — read off crm.yaml into listRecordFilters by
// gen-recordfields, never a hand-kept list) and what the store behind the seam
// can actually bind (each module's ListFilters, reaching this tool through the
// composition root). A name only one half carries is published by neither: an
// unbindable filter would run the list unnarrowed and answer a wider question
// than the caller asked, and an unpublished bindable one is capability nobody
// can reach.
//
// Metering is free here, and deliberately so: the answer is datasource.Records,
// so every row rides newWireRecord and the read bound charges per record rather
// than per call — the property A139 calls load-bearing, since an enumeration is
// the densest read a surface can offer.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// listFilter is one derived filter: the parameter name, the JSON type its
// operand takes, and the closed vocabulary when the contract declares one. The
// generated listRecordFilters table is built out of these.
type listFilter struct {
	Name string
	Type string
	Enum []string
}

// FilterVocabulary answers which filters an enumeration of one record type can
// be narrowed by — the store's half of the vocabulary.
//
// It is declared here, where it is consumed, because the answer is spread
// across modules this package may not import: the composition root implements
// it over the providers that own each record type.
type FilterVocabulary interface {
	ListFilters(t datasource.EntityType) []string
}

// listRecordTypes is the enumeration vocabulary, in the order the schema and
// the description present it. It is the set that has BOTH a contract list
// operation and a store behind the seam; `activity` is deliberately absent,
// since a timeline is reached through the record it hangs off rather than swept.
var listRecordTypes = []string{
	string(datasource.EntityContact), string(datasource.EntityCompany),
	string(datasource.EntityDeal), string(datasource.EntityLead), string(datasource.EntityProject),
}

// RegisterListTool joins list_records to the surface.
//
// The vocabulary is resolved ONCE, here, rather than per call: it is a property
// of the deployment's stores, and a schema that changed between two tools/list
// calls would make a client's cache the least trustworthy thing it holds. It is
// required rather than optional — every deployment has stores, so a missing one
// is a wiring defect, not a configuration.
func RegisterListTool(r *Registry, p datasource.SystemOfRecordProvider, vocabulary FilterVocabulary) {
	r.Register(listRecords{p: p, filters: bindableFilters(vocabulary)})
}

// bindableFilters intersects the contract's declared filters with the ones the
// stores can bind, per record type.
func bindableFilters(vocabulary FilterVocabulary) map[string][]listFilter {
	out := make(map[string][]listFilter, len(listRecordTypes))
	for _, recordType := range listRecordTypes {
		bindable := vocabulary.ListFilters(datasource.EntityType(recordType))
		for _, declared := range listRecordFilters[recordType] {
			if slices.Contains(bindable, declared.Name) {
				out[recordType] = append(out[recordType], declared)
			}
		}
	}
	return out
}

type listRecords struct {
	p datasource.SystemOfRecordProvider
	// filters is the published vocabulary per record type, already intersected.
	filters map[string][]listFilter
}

func (t listRecords) Spec() mcp.ToolSpec {
	return mcp.ToolSpec{
		Name: "list_records", Title: "List records", Version: toolVersionV1,
		Description:   listRecordsCopy.render(),
		Instead:       listRecordsCopy.Instead,
		RequiredScope: principal.ScopeRead, Tier: mcp.TierAutoExecute,
		OpenAPIOp: "listContacts/listCompanies/listDeals/listLeads/listProjects",
		InputSchema: schema(`{"type":"object","required":["record_type"],"properties":{
			"record_type":{"type":"string","enum":["contact","company","deal","lead","project"]},` +
			t.filtersProperty() + `
			"limit":{"type":"integer","minimum":1,"maximum":50},
			"cursor":{"type":"string","description":"Keyset cursor from a previous page's next_cursor"}},
			"additionalProperties":false}`),
		OutputSchema: schemaFor[SearchRecordsResult](),
	}
}

// filtersProperty declares `filters` with every published filter name as a key,
// or leaves it out when this deployment publishes none.
//
// The keys are DECLARED because a schema-constrained decoder writes no key into
// an object whose schema lists none — Gemini padded such an object with
// whitespace to the output ceiling — and `additionalProperties` does not reopen
// it. The handler still reads the same object of string operands.
//
// Each key is only a string here. Which record type takes it, and the closed
// vocabulary it takes there, stay in the description, because the names are not
// type-independent: `status` is one of open|won|lost on a deal and
// new|contacted|engaged|promoted|disqualified on a lead, so one enum per key
// would publish one of those two for both — advertising a vocabulary the handler
// then refuses, which is the specific dishonesty A139 objects to.
func (t listRecords) filtersProperty() string {
	names := t.publishedFilterNames()
	if len(names) == 0 {
		return ""
	}
	keys := make([]string, 0, len(names))
	for _, name := range names {
		keys = append(keys, strconv.Quote(name)+`:{"type":"string"}`)
	}
	return `"filters":{"type":"object","properties":{` + strings.Join(keys, ",") +
		`},"additionalProperties":false,"description":` + strconv.Quote(t.describeFilters()) + `},`
}

// publishedFilterNames returns the filter names any record type publishes, sorted.
func (t listRecords) publishedFilterNames() []string {
	seen := map[string]bool{}
	for _, filters := range t.filters {
		for _, filter := range filters {
			seen[filter.Name] = true
		}
	}
	return slices.Sorted(maps.Keys(seen))
}

// describeFilters writes the per-type vocabulary into the one place a caller
// reads it: which record type takes which key, and the operand each takes.
func (t listRecords) describeFilters() string {
	lines := make([]string, 0, len(listRecordTypes)+2)
	lines = append(lines, "Narrow the list. Every operand is a string. Each record_type takes only its own:")
	used := map[string]bool{}
	for _, recordType := range listRecordTypes {
		names := make([]string, 0, len(t.filters[recordType]))
		for _, filter := range t.filters[recordType] {
			names = append(names, filter.describe())
			used[filter.Type] = true
		}
		if len(names) == 0 {
			lines = append(lines, recordType+" — none; it can only be listed whole")
			continue
		}
		lines = append(lines, recordType+" — "+strings.Join(names, ", "))
	}
	lines = append(lines, operandKey(used)...)
	return strings.Join(append(lines, t.sourceOfStageIDs()...), " ")
}

// operandCodes abbreviates the non-string operand types, and says what each
// code means, because the code alone does not: `(a)` is a comma-separated list.
var operandCodes = []struct{ schemaType, code, meaning string }{
	{schemaArray, "(a)", "a comma-separated list"},
	{schemaBoolean, "(b)", `"true" or "false"`},
	{schemaInteger, "(i)", "a whole number"},
	{schemaNumber, "(n)", "a number"},
}

// operandKey states the meaning of each code the vocabulary uses, and nothing
// for a code it does not.
func operandKey(used map[string]bool) []string {
	var key []string
	for _, operand := range operandCodes {
		if used[operand.schemaType] {
			key = append(key, operand.code+" is "+operand.meaning)
		}
	}
	if len(key) == 0 {
		return nil
	}
	return []string{strings.Join(key, ", ") + "."}
}

// sourceOfStageIDs says where a pipeline or stage id comes from, when the
// published vocabulary asks for one.
//
// A filter naming an id nothing on the surface yields is a correct refusal that
// dead-ends, which is the defect list_pipelines exists to close;
// TestEveryToolNeedingAPipelineOrStageIDPointsAtListPipelines holds every tool
// that takes one to saying so.
func (t listRecords) sourceOfStageIDs() []string {
	for _, filters := range t.filters {
		for _, filter := range filters {
			if filter.Name == "pipeline_id" || filter.Name == "stage_id" {
				return []string{"A pipeline_id or stage_id comes from list_pipelines; " +
					"nothing else on this surface yields one."}
			}
		}
	}
	return nil
}

// describe renders one filter as a caller must spell it: the closed vocabulary
// where the contract declares one, since a plausible-looking word outside it is
// the operand a caller gets wrong, and otherwise the operand's own type where it
// is not a string — every operand travels as a string on this wire, so `true`
// and `3` are the two a caller would otherwise have to guess the spelling of.
//
// The non-string types are abbreviated to an operandCodes code, which
// operandKey explains once: the whole listing rides in every Surface-B prompt
// against a hard ceiling, so the meaning is paid once rather than per filter. A
// type with no code is spelled out, so no code is ever printed unexplained.
func (f listFilter) describe() string {
	switch {
	case len(f.Enum) > 0:
		return f.Name + " (" + strings.Join(f.Enum, "|") + ")"
	case f.Type == "" || f.Type == schemaString:
		return f.Name
	}
	for _, operand := range operandCodes {
		if operand.schemaType == f.Type {
			return f.Name + " " + operand.code
		}
	}
	return f.Name + " (" + f.Type + ")"
}

func (t listRecords) Handle(ctx context.Context, in json.RawMessage) (json.RawMessage, error) {
	var args struct {
		RecordType string            `json:"record_type"`
		Filters    map[string]string `json:"filters"`
		Limit      json.RawMessage   `json:"limit"`
		Cursor     string            `json:"cursor"`
	}
	if err := decodeArgs(in, &args); err != nil {
		return nil, err
	}
	if !slices.Contains(listRecordTypes, args.RecordType) {
		return nil, &BadArgsError{Cause: fmt.Errorf(
			"`record_type` must be one of %s", strings.Join(listRecordTypes, ", "))}
	}
	limit, err := pageLimit(args.Limit)
	if err != nil {
		return nil, err
	}
	if err := t.refuseUnaskableFilters(args.RecordType, args.Filters); err != nil {
		return nil, err
	}
	res, err := t.p.Search(ctx, datasource.SearchQuery{
		EntityTypes: []datasource.EntityType{datasource.EntityType(args.RecordType)},
		Filters:     args.Filters,
		Limit:       limit,
		Cursor:      args.Cursor,
	})
	if err != nil {
		return nil, err
	}
	return json.Marshal(searchResult(ctx, res))
}

// pageLimit reads the page size, telling an ABSENT argument from an explicit
// null.
//
// It is raw rather than an int for the reason the query grammar spells out
// about its own `limit`: encoding/json gives both the zero value, and the two
// mean different things. Absent asks for the contract's default; null is a page
// size the schema does not have, and reading it as absent would serve a page
// nobody asked for while reporting success.
func pageLimit(raw json.RawMessage) (int, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return 0, nil
	}
	if bytes.Equal(trimmed, []byte("null")) {
		return 0, &BadArgsError{Cause: errors.New(
			"`limit` is null; send a page size between 1 and 50, or leave the argument out for the default")}
	}
	var limit int
	if err := json.Unmarshal(trimmed, &limit); err != nil {
		return 0, &BadArgsError{Cause: errors.New("`limit` takes a whole number between 1 and 50")}
	}
	return limit, nil
}

// refuseUnaskableFilters holds the call to the vocabulary this deployment
// publishes for that record type.
//
// The refusal NAMES what may be asked instead, because the alternative — "that
// filter is not valid" — sends a caller back to guess a second name from the
// same empty set. A filter a type does not carry is refused rather than
// dropped, for the reason the whole path exists: a dropped filter answers a
// wider question in the shape of the right answer.
func (t listRecords) refuseUnaskableFilters(recordType string, filters map[string]string) error {
	published := t.filters[recordType]
	for _, name := range slices.Sorted(maps.Keys(filters)) {
		index := slices.IndexFunc(published, func(f listFilter) bool { return f.Name == name })
		if index < 0 {
			return &BadArgsError{Cause: fmt.Errorf("a %s cannot be listed by %q; this deployment lists it by %s",
				recordType, name, describeNames(published))}
		}
		if err := refuseUnaskableOperand(published[index], filters[name]); err != nil {
			return err
		}
	}
	return nil
}

// refuseUnaskableOperand holds an operand to the closed vocabulary its filter
// declares. It says nothing about a filter that declares none: an unmatched
// free-text value selects no rows, which is the honest answer to it, and the
// seam parses the reference and flag shapes when it binds them.
func refuseUnaskableOperand(filter listFilter, operand string) error {
	if len(filter.Enum) == 0 || slices.Contains(filter.Enum, operand) {
		return nil
	}
	// Same split as refuseUnimportableObject: the operand the caller sent is
	// bounded in Cause and says WHICH value was refused, and the closed set is
	// ours in Guidance so a long operand cannot spend its budget.
	return &BadArgsError{
		Cause:    fmt.Errorf("%s %q is not a value it takes", filter.Name, operand),
		Field:    filter.Name,
		Guidance: filter.Name + " is one of: " + strings.Join(filter.Enum, ", "),
	}
}

// describeNames lists a record type's filters for a refusal, or says plainly
// that it has none rather than printing an empty list.
func describeNames(filters []listFilter) string {
	if len(filters) == 0 {
		return "nothing — it can only be listed whole"
	}
	names := make([]string, 0, len(filters))
	for _, filter := range filters {
		names = append(names, filter.Name)
	}
	return strings.Join(names, ", ")
}
