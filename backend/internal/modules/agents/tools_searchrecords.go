// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// search_records runs the ranked engine GET /v1/search runs, so one query finds
// the same records in the same order on both doors. The engine answers refs;
// each is read back through the provider, which keeps fields, version, owner,
// trust tier and evidence as read_record serves them.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// RecordSearch runs the cross-object ranked search. compose binds it to the
// search module's engine, which this module may not import.
type RecordSearch func(ctx context.Context, q RecordSearchQuery) (RecordSearchPage, error)

// RecordSearchQuery is one ranked search. A nil PerType asks for the ranked
// list; a set one asks for a page grouped by type.
type RecordSearchQuery struct {
	Text          string
	Types         []string
	Limit         int
	Cursor        string
	PerType       *int
	WithEmployees bool
}

// RecordSearchPage is one page of the engine's answer, in its ranked order.
// TypesWithMore is non-nil on a grouped page alone.
type RecordSearchPage struct {
	Hits          []RecordSearchHit
	NextCursor    string
	TypesWithMore []string
}

// RecordSearchHit is one ranked ref and why it matched.
type RecordSearchHit struct {
	Ref   datasource.EntityRef
	Match RecordMatch
}

// RecordMatch says why a record matched: the excerpt the engine ranked, and
// the company a contact works at when the employer arm found it.
type RecordMatch struct {
	Snippet string          `json:"snippet,omitempty"`
	WorksAt *RecordEmployer `json:"works_at,omitempty"`
}

// RecordEmployer is the matched company a contact was found through.
type RecordEmployer struct {
	CompanyID   ids.UUID `json:"company_id"`
	CompanyName string   `json:"company_name"`
}

// SearchRecordsResult is what search_records answers: the records the ranked
// search found, best first, each with why it matched.
type SearchRecordsResult struct {
	Records    []searchedRecord `json:"records"`
	NextCursor string           `json:"next_cursor,omitempty"`
	// TypesWithMore is present on a grouped (per_type) page alone, and empty
	// there when every type fit.
	TypesWithMore *[]string `json:"types_with_more,omitempty"`
	// Coverage is complete_exact only on a page holding every match, each read
	// back. Not omitempty: an absent coverage would read as a complete one.
	Coverage string `json:"coverage"`
	// Notes say why coverage is lowered, never null on the wire.
	Notes []QueryNote `json:"notes"`
}

// CodeMoreRecordsMatch is search_records' note for a page with a remainder:
// more records match than it holds.
const CodeMoreRecordsMatch = "more_records_match"

// searchedRecord is a ranked record and why it matched.
type searchedRecord struct {
	recordWithOwner
	// Match is absent when the engine has no excerpt and no employer to show.
	Match *RecordMatch `json:"match,omitempty"`
}

// recordSearchTypes are the types search_records reaches. tag is left out:
// read_record cannot read a tag, and read_tags is the door to that vocabulary.
var recordSearchTypes = []string{
	string(datasource.EntityContact), string(datasource.EntityCompany), string(datasource.EntityDeal),
	string(datasource.EntityLead), string(datasource.EntityProject), string(datasource.EntityProduct),
	string(datasource.EntityOfferTemplate), string(datasource.EntityActivity),
}

// recordSearchDefaultTypes are searched when the caller names none. The
// timeline and the catalog are searched only when named.
func recordSearchDefaultTypes() []string {
	out := []string{}
	for _, t := range datasource.RecordTypes() {
		out = append(out, string(t))
	}
	return out
}

// RegisterRecordSearchTool joins search_records to the surface once an engine
// is bound, as search_context waits for its retriever.
func RegisterRecordSearchTool(r *Registry, p datasource.SystemOfRecordProvider, engine RecordSearch) {
	if engine == nil {
		return
	}
	r.Register(searchRecords{p: p, engine: engine, name: r.seats})
}

type searchRecords struct {
	p      datasource.SystemOfRecordProvider
	engine RecordSearch
	name   SeatNamer
}

func (t searchRecords) Spec() mcp.ToolSpec {
	types := `["` + strings.Join(recordSearchTypes, `","`) + `"]`
	return mcp.ToolSpec{
		Name: "search_records", Title: "Search records", Version: toolVersionV1,
		Description:   searchRecordsCopy.render(),
		Instead:       searchRecordsCopy.Instead,
		RequiredScope: principal.ScopeRead, Tier: mcp.TierAutoExecute,
		Unlisted:  true,
		OpenAPIOp: "search",
		InputSchema: schema(fmt.Sprintf(`{"type":"object","required":["q"],"properties":{
			"q":{"type":"string","description":"The words to match against the text stored on each record."},
			"record_type":{"type":"string","enum":%[1]s,"description":"Search one type."},
			"record_types":{"type":"array","items":{"type":"string","enum":%[1]s},"description":"Search these types, in place of record_type. Omit both for contact, company, deal, lead and project."},
			"with_employees":{"type":"boolean","description":"Also find the contacts who work at a matching company; each carries match.works_at."},
			"per_type":{"type":"integer","minimum":1,"maximum":20,"description":"Answer up to this many of each type instead of one ranked list. Takes no cursor or limit; types_with_more names the types holding more."},
			"limit":{"type":"integer","minimum":1,"maximum":50},
			"cursor":{"type":"string","description":"next_cursor from the previous page."}},
			"additionalProperties":false}`, types)),
		OutputSchema: schemaFor[SearchRecordsResult](),
	}
}

// CoverageClasses: a page holding every match, or part of them. Never
// ranked_semantic — the engine ranks on words, not meaning.
func (t searchRecords) CoverageClasses() []string {
	return []string{CoverageCompleteExact, CoveragePartialDegraded}
}

type searchRecordsArgs struct {
	Q             string   `json:"q"`
	RecordType    string   `json:"record_type"`
	RecordTypes   []string `json:"record_types"`
	WithEmployees bool     `json:"with_employees"`
	PerType       *int     `json:"per_type"`
	Limit         int      `json:"limit"`
	Cursor        string   `json:"cursor"`
}

func (t searchRecords) Handle(ctx context.Context, in json.RawMessage) (json.RawMessage, error) {
	var args searchRecordsArgs
	if err := decodeArgs(in, &args); err != nil {
		return nil, err
	}
	q, err := args.query(ctx)
	if err != nil {
		return nil, err
	}
	page, err := t.engine(ctx, q)
	if err != nil {
		return nil, err
	}
	result, err := t.hydrate(ctx, page)
	if err != nil {
		return nil, err
	}
	return json.Marshal(result)
}

// query turns the arguments into one engine query. It refuses a search with no
// letter or digit, which holds no word to match. It refuses a named type the caller may not read, which the engine would
// drop in silence. The registry holds the type enum.
func (a searchRecordsArgs) query(ctx context.Context) (RecordSearchQuery, error) {
	if !strings.ContainsFunc(a.Q, func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }) {
		return RecordSearchQuery{}, &BadArgsError{Field: "q", Cause: errors.New(
			"`q` takes the words to search for; list_records lists records without words")}
	}
	if a.RecordType != "" && len(a.RecordTypes) > 0 {
		return RecordSearchQuery{}, &BadArgsError{
			Field: "record_types", Cause: errors.New("name one record_type or a list of record_types, not both"),
		}
	}
	named := a.RecordTypes
	if a.RecordType != "" {
		named = []string{a.RecordType}
	}
	for _, recordType := range named {
		if err := auth.Require(ctx, recordType, principal.ActionRead); err != nil {
			return RecordSearchQuery{}, err
		}
	}
	if len(named) == 0 {
		named = recordSearchDefaultTypes()
	}
	return RecordSearchQuery{
		Text: a.Q, Types: named, Limit: a.Limit, Cursor: a.Cursor,
		PerType: a.PerType, WithEmployees: a.WithEmployees,
	}, nil
}

// hydrate reads each hit back in the engine's order. A hit gone or out of
// scope since it ranked is dropped, and the notes say so without a count.
func (t searchRecords) hydrate(ctx context.Context, page RecordSearchPage) (SearchRecordsResult, error) {
	wires := make([]wireRecord, 0, len(page.Hits))
	matches := make([]*RecordMatch, 0, len(page.Hits))
	var dropped bool
	for _, hit := range page.Hits {
		record, readable, err := readServable(ctx, t.p, hit.Ref)
		if err != nil {
			return SearchRecordsResult{}, err
		}
		if !readable {
			dropped = true
			continue
		}
		wires = append(wires, newWireRecord(ctx, record))
		var match *RecordMatch
		if hit.Match != (RecordMatch{}) {
			match = &hit.Match
		}
		matches = append(matches, match)
	}
	owned := withOwners(ctx, t.name, wires)
	result := SearchRecordsResult{
		Records: make([]searchedRecord, len(owned)), NextCursor: page.NextCursor,
		Coverage: CoverageCompleteExact, Notes: []QueryNote{},
	}
	for i, row := range owned {
		result.Records[i] = searchedRecord{recordWithOwner: row, Match: matches[i]}
	}
	if page.TypesWithMore != nil {
		result.TypesWithMore = &page.TypesWithMore
	}
	if more := remainderNote(page); more != nil {
		result.Coverage = CoveragePartialDegraded
		result.Notes = append(result.Notes, *more)
	}
	if dropped {
		result.Coverage = CoveragePartialDegraded
		result.Notes = append(result.Notes, QueryNote{
			Code: CodeRowUnreadable,
			// No count: how many records a caller may not read is what
			// existence-hiding withholds.
			Detail: "at least one ranked record could not be read back and is not among these records; " +
				"the ranking goes on past it, so continue with next_cursor when this page has one",
		})
	}
	return result, nil
}

// remainderNote says where the matches past this page are, or nil when the
// page holds them all.
func remainderNote(page RecordSearchPage) *QueryNote {
	switch {
	case page.NextCursor != "":
		return &QueryNote{Code: CodeMoreRecordsMatch, Detail: "more records match than this page holds; continue with next_cursor"}
	case len(page.TypesWithMore) > 0:
		return &QueryNote{Code: CodeMoreRecordsMatch, Detail: "the types in types_with_more hold more matches than " +
			"this page shows; search one of them with record_type to page through it"}
	default:
		return nil
	}
}
