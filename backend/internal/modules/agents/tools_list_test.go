// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"context"
	"encoding/json"
	"maps"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// The published vocabulary is the INTERSECTION of what the contract declares
// and what a store can bind.
//
// `tag` is the live case and the reason this matters: listContacts declares it
// and no column here holds tags, so publishing it would offer a narrowing that
// runs the list whole. A filter only one half carries is offered by neither.
func TestOnlyAFilterBothTheContractAndAStoreCarryIsPublished(t *testing.T) {
	tool := listRecords{filters: bindableFilters(probeVocabulary{})}

	contact := filterNamesOf(tool, "contact")
	if slices.Contains(contact, "tag") {
		t.Errorf("contact publishes %v, which includes a filter no store binds — the list would run whole", contact)
	}
	if !slices.Contains(contact, "owner_id") {
		t.Errorf("contact publishes %v, which drops a filter both halves carry", contact)
	}
	if deal := filterNamesOf(tool, "deal"); !slices.Contains(deal, "stage_id") {
		t.Errorf("deal publishes %v, want the contract's own stage_id", deal)
	}
}

// A record type this surface does not enumerate is refused BEFORE the seam is
// touched. Passing it on would ask the composite provider for a type it answers
// with an unsupported-entity error, which reads to a caller as a fault rather
// than as a name they may not use.
func TestARecordTypeOutsideTheEnumerationIsRefused(t *testing.T) {
	seam := &listProbeProvider{}
	tool := listRecords{p: seam, filters: bindableFilters(probeVocabulary{})}

	_, err := tool.Handle(t.Context(), json.RawMessage(`{"record_type":"activity"}`))

	if err == nil {
		t.Fatal("an unlistable record type was accepted")
	}
	if seam.queries != nil {
		t.Errorf("the call reached the seam before it was refused: %+v", seam.queries)
	}
	for _, want := range []string{"contact", "deal"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal never says what may be listed instead: %v", err)
		}
	}
}

// A filter the type does not publish is refused, and the refusal NAMES what may
// be asked instead — "not a valid filter" sends a caller back to guess a second
// name out of the same empty set.
func TestAFilterTheTypeDoesNotCarryIsRefusedByName(t *testing.T) {
	seam := &listProbeProvider{}
	tool := listRecords{p: seam, filters: bindableFilters(probeVocabulary{})}

	_, err := tool.Handle(t.Context(), json.RawMessage(`{"record_type":"contact","filters":{"stage_id":"x"}}`))

	if err == nil {
		t.Fatal("a contact was listed by a filter only a deal carries")
	}
	if !strings.Contains(err.Error(), "stage_id") || !strings.Contains(err.Error(), "owner_id") {
		t.Errorf("the refusal names neither what was refused nor what may be asked: %v", err)
	}
	if seam.queries != nil {
		t.Error("the unaskable filter reached the seam, where it would have run the list unnarrowed")
	}
}

// An operand outside a filter's declared vocabulary is refused with that
// vocabulary. The contract publishes the words; a caller guessing a plausible
// one ("closed") gets the list of real ones rather than an empty page.
func TestAnOperandOutsideItsDeclaredVocabularyIsRefused(t *testing.T) {
	tool := listRecords{p: &listProbeProvider{}, filters: bindableFilters(probeVocabulary{})}

	_, err := tool.Handle(t.Context(), json.RawMessage(`{"record_type":"deal","filters":{"status":"closed"}}`))

	if err == nil {
		t.Fatal("a status outside the contract's enum was accepted")
	}
	if !strings.Contains(err.Error(), "won") {
		t.Errorf("the refusal does not say which words the filter takes: %v", err)
	}
}

// What the caller asked for reaches the seam intact: the one entity type, the
// filters, the page size and the cursor. A tool that dropped any of them would
// answer a different question in the shape of this one.
func TestTheCallReachesTheSeamAsItWasAsked(t *testing.T) {
	owner := ids.NewV7()
	seam := &listProbeProvider{}
	tool := listRecords{p: seam, filters: bindableFilters(probeVocabulary{})}

	_, err := tool.Handle(t.Context(), json.RawMessage(
		`{"record_type":"deal","filters":{"owner_id":"`+owner.String()+`","status":"open"},"limit":25,"cursor":"c1"}`))
	if err != nil {
		t.Fatalf("listing deals: %v", err)
	}

	if len(seam.queries) != 1 {
		t.Fatalf("the seam saw %d queries, want one", len(seam.queries))
	}
	q := seam.queries[0]
	if len(q.EntityTypes) != 1 || q.EntityTypes[0] != datasource.EntityDeal {
		t.Errorf("entity types = %v, want exactly [deal]", q.EntityTypes)
	}
	if q.Filters["owner_id"] != owner.String() || q.Filters["status"] != "open" {
		t.Errorf("filters = %v, want both carried verbatim", q.Filters)
	}
	if q.Limit != 25 || q.Cursor != "c1" {
		t.Errorf("limit/cursor = %d/%q, want 25/c1", q.Limit, q.Cursor)
	}
	if q.Text != "" {
		t.Errorf("the query carries text %q — an enumeration asks for no term, and a term would narrow it", q.Text)
	}
}

// An enumeration is charged PER RECORD, not per call. It is the densest read a
// surface can offer, so metering it by the call would make it the cheapest way
// to read a workspace — the exact failure A139 names.
func TestAnEnumerationIsChargedPerRecord(t *testing.T) {
	seam := &listProbeProvider{}
	for range 3 {
		seam.records = append(seam.records, recordAt(datasource.EntityDeal,
			time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC), true))
	}
	registry, charger, ctx := chargingRegistry(t, listRecords{p: seam, filters: bindableFilters(probeVocabulary{})})

	if _, err := registry.Invoke(ctx, "list_records", json.RawMessage(`{"record_type":"deal"}`)); err != nil {
		t.Fatalf("invoking list_records: %v", err)
	}

	if charger.reads() != 3 {
		t.Errorf("charged %d for a page of 3 records, want 3", charger.reads())
	}
}

// `filters` DECLARES its keys, and they are exactly the names some record type
// publishes — both sides of one wire. A schema-constrained decoder writes no key
// into an object whose schema lists none, so an undeclared name is one a
// constrained model can never send; a declared name no type takes is a key the
// handler refuses.
func TestTheFiltersObjectDeclaresEveryPublishedNameAndNothingElse(t *testing.T) {
	seam := &listProbeProvider{}
	tool := listRecords{p: seam, filters: bindableFilters(probeVocabulary{})}
	var declared struct {
		Properties struct {
			Filters struct {
				Type                 string                     `json:"type"`
				Properties           map[string]json.RawMessage `json:"properties"`
				AdditionalProperties *bool                      `json:"additionalProperties"` //nolint:tagliatelle // JSON Schema's own key spelling
			} `json:"filters"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(tool.Spec().InputSchema, &declared); err != nil {
		t.Fatalf("list_records' input schema does not parse: %v", err)
	}
	filters := declared.Properties.Filters
	if filters.Type != "object" || filters.AdditionalProperties == nil || *filters.AdditionalProperties {
		t.Errorf("filters must be a closed object; got type %q, additionalProperties %v", filters.Type, filters.AdditionalProperties)
	}

	published := map[string]string{}
	for _, recordType := range listRecordTypes {
		for _, filter := range tool.filters[recordType] {
			published[filter.Name] = recordType
		}
	}
	if got, want := slices.Sorted(maps.Keys(filters.Properties)), slices.Sorted(maps.Keys(published)); !slices.Equal(got, want) {
		t.Errorf("filters declares %v; the published vocabulary is %v", got, want)
	}
	for name, recordType := range published {
		operand := "x"
		if filter := tool.filters[recordType][slices.IndexFunc(tool.filters[recordType],
			func(f listFilter) bool { return f.Name == name })]; len(filter.Enum) > 0 {
			operand = filter.Enum[0]
		}
		call := `{"record_type":"` + recordType + `","filters":{"` + name + `":"` + operand + `"}}`
		if _, err := tool.Handle(t.Context(), json.RawMessage(call)); err != nil {
			t.Errorf("the schema declares filter %q and the handler refuses it on a %s: %v", name, recordType, err)
		}
	}
}

// A deployment that publishes no filter declares no `filters` at all, rather
// than an object with no keys a constrained decoder could write.
func TestADeploymentWithNoFiltersDeclaresNoFiltersObject(t *testing.T) {
	var declared struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	tool := listRecords{filters: bindableFilters(noFilterVocabulary{})}
	if err := json.Unmarshal(tool.Spec().InputSchema, &declared); err != nil {
		t.Fatalf("list_records' input schema does not parse: %v", err)
	}
	if raw, present := declared.Properties["filters"]; present {
		t.Errorf("with nothing published, filters is still declared: %s", raw)
	}
}

// The description says which filters each type takes, because the schema's
// keys cannot: `status` is open|won|lost on a deal and new|contacted|engaged on
// a lead, so one enum per key would publish one type's vocabulary for both.
func TestTheDescriptionCarriesThePerTypeVocabulary(t *testing.T) {
	described := listRecords{filters: bindableFilters(probeVocabulary{})}.describeFilters()
	// Anchored on the type heading and on the filters themselves, not on which
	// one happens to sort first: the set is alphabetical, so pinning the first
	// entry fails the day a type gains a filter earlier in the alphabet —
	// which says nothing about whether the vocabulary is published correctly.
	for _, want := range []string{
		"contact — ", "owner_id", "deal — ", "stage_id", "open|won|lost",
	} {
		if !strings.Contains(described, want) {
			t.Errorf("the filter description lacks %q:\n%s", want, described)
		}
	}
}

// A type whose stores bind nothing says so, rather than presenting an empty
// list a caller has to interpret.
func TestATypeWithNoBindableFilterSaysItListsWhole(t *testing.T) {
	tool := listRecords{filters: bindableFilters(noFilterVocabulary{})}
	if described := tool.describeFilters(); !strings.Contains(described, "none; it can only be listed whole") {
		t.Errorf("a type with no filters reads as an omission:\n%s", described)
	}
}

// --- probes ---

// probeVocabulary answers the store half of the vocabulary the way the
// composite provider does: everything the contract declares EXCEPT the three
// parameters no store binds today (contact.tag, company.domain,
// lead.min_score).
type probeVocabulary struct{}

func (probeVocabulary) ListFilters(t datasource.EntityType) []string {
	unbindable := map[string]bool{"tag": true, "domain": true, "min_score": true}
	var names []string
	for _, declared := range listRecordFilters[string(t)] {
		if !unbindable[declared.Name] {
			names = append(names, declared.Name)
		}
	}
	return names
}

type noFilterVocabulary struct{}

func (noFilterVocabulary) ListFilters(datasource.EntityType) []string { return nil }

func filterNamesOf(t listRecords, recordType string) []string {
	names := make([]string, 0, len(t.filters[recordType]))
	for _, filter := range t.filters[recordType] {
		names = append(names, filter.Name)
	}
	return names
}

// listProbeProvider records the queries it was asked and answers a fixed page.
// Only Search is implemented — the embedded nil interface is this tree's way of
// saying anything else is outside the probe's contract, and a call to one
// panics rather than passing quietly.
type listProbeProvider struct {
	datasource.SystemOfRecordProvider
	records []datasource.Record
	queries []datasource.SearchQuery
}

func (p *listProbeProvider) Search(_ context.Context, q datasource.SearchQuery) (datasource.SearchResult, error) {
	p.queries = append(p.queries, q)
	return datasource.SearchResult{Records: p.records}, nil
}

// An explicit null page size is REFUSED rather than read as an omission.
//
// encoding/json gives an absent `limit` and a null one the same zero value, and
// the two mean different things: absent asks for the contract's default, while
// null is a page size this schema does not have. Serving the default for it
// would answer a page nobody asked for and report success.
func TestANullPageSizeIsRefusedRatherThanDefaulted(t *testing.T) {
	seam := &listProbeProvider{}
	tool := listRecords{p: seam, filters: bindableFilters(probeVocabulary{})}

	_, err := tool.Handle(t.Context(), json.RawMessage(`{"record_type":"deal","limit":null}`))

	if err == nil {
		t.Fatal("a null limit was served as the default page")
	}
	if !strings.Contains(err.Error(), "limit") {
		t.Errorf("the refusal never names the argument: %v", err)
	}
	if seam.queries != nil {
		t.Error("the call reached the seam before its page size was refused")
	}
}

// An omitted page size still asks for the default — the argument is optional,
// and refusing null must not make it required.
func TestAnOmittedPageSizeStillAsksForTheDefault(t *testing.T) {
	seam := &listProbeProvider{}
	tool := listRecords{p: seam, filters: bindableFilters(probeVocabulary{})}

	if _, err := tool.Handle(t.Context(), json.RawMessage(`{"record_type":"deal"}`)); err != nil {
		t.Fatalf("listing with no limit: %v", err)
	}
	if len(seam.queries) != 1 || seam.queries[0].Limit != 0 {
		t.Errorf("the seam saw %+v, want one query asking for the default page size", seam.queries)
	}
}
