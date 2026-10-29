// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// rankingProbe is a ranked engine that answers a fixed page and keeps the
// query it was asked.
type rankingProbe struct {
	page  RecordSearchPage
	err   error
	asked *RecordSearchQuery
}

func (p *rankingProbe) search(_ context.Context, q RecordSearchQuery) (RecordSearchPage, error) {
	p.asked = &q
	return p.page, p.err
}

// searchReader is an agent that may read the named objects, every row of them.
func searchReader(objects ...string) context.Context {
	grants := map[string]principal.ObjectGrant{}
	for _, object := range objects {
		grants[object] = principal.ObjectGrant{Read: true}
	}
	return principal.WithActor(principal.WithWorkspaceID(context.Background(), ids.NewV7()), principal.Principal{
		Type: principal.PrincipalAgent, ID: "agent:search",
		Permissions: principal.Permissions{Objects: grants, RowScope: principal.RowScopeAll},
		Scopes:      principal.NewScopeSet(principal.ScopeRead),
	})
}

func searchRecordsOver(provider datasource.SystemOfRecordProvider, engine *rankingProbe) searchRecords {
	return searchRecords{p: provider, engine: engine.search}
}

func searchedPage(t *testing.T, raw json.RawMessage) SearchRecordsResult {
	t.Helper()
	var page SearchRecordsResult
	if err := json.Unmarshal(raw, &page); err != nil {
		t.Fatalf("decoding the answer %s: %v", raw, err)
	}
	return page
}

func TestASearchWithoutWordsIsSentToListRecords(t *testing.T) {
	engine := &rankingProbe{}
	tool := searchRecordsOver(nil, engine)
	for _, args := range []string{`{"q":""}`, `{"q":"   ","record_type":"deal"}`, `{"q":" ?! "}`} {
		_, err := tool.Handle(searchReader("deal"), json.RawMessage(args))
		badArgs, ok := errors.AsType[*BadArgsError](err)
		if !ok || badArgs.Field != "q" || !strings.Contains(badArgs.Error(), "list_records") {
			t.Errorf("%s → %v, want a refusal of `q` naming list_records", args, err)
		}
	}
	if engine.asked != nil {
		t.Errorf("the engine ran for a search with no words: %+v", *engine.asked)
	}
}

// A grouped page holds up to per_type of each type searched, and every hit is
// read back, so per_type is bounded by the page's 25 records.
func TestAGroupedPageNeverReadsBackMoreThanAPage(t *testing.T) {
	for _, tc := range []struct {
		args  string
		admit bool
	}{
		{`{"q":"Acme","per_type":5}`, true},
		{`{"q":"Acme","per_type":6}`, false},
		{`{"q":"Acme","record_type":"deal","per_type":20}`, true},
		{`{"q":"Acme","record_types":["contact","company","deal","lead","project","product","offer_template","activity"],"per_type":3}`, true},
		{`{"q":"Acme","record_types":["contact","company","deal","lead","project","product","offer_template","activity"],"per_type":4}`, false},
	} {
		engine := &rankingProbe{}
		_, err := searchRecordsOver(nil, engine).Handle(searchReader(recordSearchTypes...), json.RawMessage(tc.args))
		if tc.admit {
			if err != nil {
				t.Errorf("%s was refused: %v", tc.args, err)
			}
			continue
		}
		badArgs, ok := errors.AsType[*BadArgsError](err)
		if !ok || badArgs.Field != "per_type" || !strings.Contains(badArgs.Error(), "25") {
			t.Errorf("%s → %v, want a refusal of per_type naming the 25-record page", tc.args, err)
		}
		if engine.asked != nil {
			t.Errorf("%s reached the engine", tc.args)
		}
	}
}

func TestOneTypeAndAListOfTypesAreNotBothTaken(t *testing.T) {
	_, err := searchRecordsOver(nil, &rankingProbe{}).Handle(searchReader("deal", "lead"),
		json.RawMessage(`{"q":"Acme","record_type":"deal","record_types":["lead"]}`))
	if badArgs, ok := errors.AsType[*BadArgsError](err); !ok || badArgs.Field != "record_types" {
		t.Errorf("both type arguments → %v, want a refusal of record_types", err)
	}
}

// The engine leaves a denied type out of its answer. A caller who named that
// type would read the empty page as "nothing matches", so it hears the denial.
func TestANamedTypeTheCallerCannotReadIsRefused(t *testing.T) {
	engine := &rankingProbe{}
	_, err := searchRecordsOver(nil, engine).Handle(searchReader("contact"),
		json.RawMessage(`{"q":"Acme","record_types":["contact","deal"]}`))
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("naming a type the caller cannot read → %v, want ErrPermissionDenied", err)
	}
	if engine.asked != nil {
		t.Error("the engine ran for a type the caller cannot read")
	}
}

func TestExcludedIdsReachTheEngineAndABadOneIsRefused(t *testing.T) {
	skip := ids.NewV7()
	engine := &rankingProbe{}
	if _, err := searchRecordsOver(nil, engine).Handle(searchReader("contact"),
		json.RawMessage(`{"q":"Acme","exclude_ids":["`+skip.String()+`"]}`)); err != nil {
		t.Fatal(err)
	}
	if got := engine.asked.Exclude; len(got) != 1 || got[0] != skip {
		t.Errorf("the engine was asked to exclude %v, want [%v]", got, skip)
	}
	_, err := searchRecordsOver(nil, &rankingProbe{}).Handle(searchReader("contact"),
		json.RawMessage(`{"q":"Acme","exclude_ids":["not-an-id"]}`))
	if badArgs, ok := errors.AsType[*BadArgsError](err); !ok || badArgs.Field != "exclude_ids" {
		t.Errorf("a malformed id → %v, want a refusal of exclude_ids", err)
	}
}

func TestTheArgumentsReachTheEngine(t *testing.T) {
	perType := 3
	for _, tc := range []struct {
		args string
		want RecordSearchQuery
	}{
		{
			`{"q":"Acme"}`,
			RecordSearchQuery{Text: "Acme", Types: []string{"contact", "company", "deal", "lead", "project"}, Limit: 20},
		},
		{
			`{"q":"Acme","record_type":"activity","limit":5,"cursor":"c1"}`,
			RecordSearchQuery{Text: "Acme", Types: []string{"activity"}, Limit: 5, Cursor: "c1"},
		},
		{
			`{"q":"Acme","record_types":["product","offer_template"],"per_type":3,"with_employees":true}`,
			RecordSearchQuery{Text: "Acme", Types: []string{"product", "offer_template"}, PerType: &perType, WithEmployees: true},
		},
	} {
		engine := &rankingProbe{}
		ctx := searchReader("activity", "product", "offer_template")
		if _, err := searchRecordsOver(nil, engine).Handle(ctx, json.RawMessage(tc.args)); err != nil {
			t.Fatalf("%s: %v", tc.args, err)
		}
		got := *engine.asked
		samePerType := (got.PerType == nil) == (tc.want.PerType == nil) &&
			(got.PerType == nil || *got.PerType == *tc.want.PerType)
		if got.Text != tc.want.Text || !slices.Equal(got.Types, tc.want.Types) || got.Limit != tc.want.Limit ||
			got.Cursor != tc.want.Cursor || got.WithEmployees != tc.want.WithEmployees || !samePerType {
			t.Errorf("%s asked the engine %+v, want %+v", tc.args, got, tc.want)
		}
	}
}

// Hits are read back in the engine's order. One gone since it ranked is left
// out, and the rest keep their places and their match.
func TestAHitThatCannotBeReadBackIsDropped(t *testing.T) {
	first, gone, last := ids.NewV7(), ids.NewV7(), ids.NewV7()
	company, synced := ids.NewV7(), time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	provider := &queryProbeProvider{records: map[ids.UUID]datasource.Record{
		first: recordAt(datasource.EntityContact, synced, true),
		last:  recordAt(datasource.EntityCompany, synced, true),
	}}
	engine := &rankingProbe{page: RecordSearchPage{NextCursor: "next", Hits: []RecordSearchHit{
		{
			Ref:   datasource.EntityRef{Type: datasource.EntityContact, ID: first},
			Match: RecordMatch{WorksAt: &RecordEmployer{CompanyID: company, CompanyName: "Acme"}},
		},
		{Ref: datasource.EntityRef{Type: datasource.EntityDeal, ID: gone}, Match: RecordMatch{Snippet: "Acme renewal"}},
		{Ref: datasource.EntityRef{Type: datasource.EntityCompany, ID: last}},
	}}}
	raw, err := searchRecordsOver(provider, engine).Handle(searchReader(), json.RawMessage(`{"q":"Acme"}`))
	if err != nil {
		t.Fatal(err)
	}
	page := searchedPage(t, raw)
	if len(page.Records) != 2 || page.Records[0].ID != first || page.Records[1].ID != last {
		t.Fatalf("the answer is %s, want the contact then the company, without the deal gone since it ranked", raw)
	}
	if works := page.Records[0].Match; works == nil || works.WorksAt == nil || works.WorksAt.CompanyID != company {
		t.Errorf("the contact found through its employer does not say where it works: %s", raw)
	}
	if page.Records[1].Match != nil {
		t.Errorf("a hit with no excerpt and no employer carries a match: %s", raw)
	}
	if page.NextCursor != "next" || page.TypesWithMore != nil {
		t.Errorf("a ranked page answered cursor %q and types_with_more %v", page.NextCursor, page.TypesWithMore)
	}
	if page.Coverage != CoveragePartialDegraded {
		t.Errorf("coverage = %q, want %q — a page missing a ranked record is not whole", page.Coverage, CoveragePartialDegraded)
	}
	var dropped *QueryNote
	for i := range page.Notes {
		if page.Notes[i].Code == CodeRowUnreadable {
			dropped = &page.Notes[i]
		}
	}
	if dropped == nil || !strings.Contains(dropped.Detail, "next_cursor") || strings.Contains(dropped.Detail, "search again") {
		t.Errorf("the dropped hit is reported as %+v, want a row_unreadable note sending the caller on with next_cursor", page.Notes)
	}
}

// Coverage is complete only on a page holding every match; a remainder past it
// lowers it, with a note saying where the rest is.
func TestCoverageSaysWhetherMoreRecordsMatch(t *testing.T) {
	hit := ids.NewV7()
	provider := &queryProbeProvider{records: map[ids.UUID]datasource.Record{
		hit: recordAt(datasource.EntityContact, time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC), true),
	}}
	hits := []RecordSearchHit{{Ref: datasource.EntityRef{Type: datasource.EntityContact, ID: hit}}}
	for _, tc := range []struct {
		what string
		page RecordSearchPage
		args string
		want string
	}{
		{"the last page", RecordSearchPage{Hits: hits}, `{"q":"Acme"}`, CoverageCompleteExact},
		{"a page with more", RecordSearchPage{Hits: hits, NextCursor: "next"}, `{"q":"Acme"}`, CoveragePartialDegraded},
		{"a grouped page with every type whole", RecordSearchPage{Hits: hits, TypesWithMore: []string{}}, `{"q":"Acme","per_type":2}`, CoverageCompleteExact},
		{"a grouped page with a type holding more", RecordSearchPage{Hits: hits, TypesWithMore: []string{"contact"}}, `{"q":"Acme","per_type":2}`, CoveragePartialDegraded},
	} {
		raw, err := searchRecordsOver(provider, &rankingProbe{page: tc.page}).Handle(searchReader(), json.RawMessage(tc.args))
		if err != nil {
			t.Fatalf("%s: %v", tc.what, err)
		}
		page := searchedPage(t, raw)
		if page.Coverage != tc.want {
			t.Errorf("%s: coverage = %q, want %q", tc.what, page.Coverage, tc.want)
		}
		if more := hasNote(page.Notes, CodeMoreRecordsMatch); more != (tc.want == CoveragePartialDegraded) {
			t.Errorf("%s: notes %+v, want the more-records note exactly when coverage is lowered", tc.what, page.Notes)
		}
	}
}

// A store that cannot answer is a fault, not a short page.
func TestAFailedReadBackFailsTheSearch(t *testing.T) {
	hit := ids.NewV7()
	unreachable := errors.New("the store is unreachable")
	provider := &queryProbeProvider{fail: map[ids.UUID]error{hit: unreachable}}
	engine := &rankingProbe{page: RecordSearchPage{Hits: []RecordSearchHit{
		{Ref: datasource.EntityRef{Type: datasource.EntityContact, ID: hit}},
	}}}
	if _, err := searchRecordsOver(provider, engine).Handle(searchReader(), json.RawMessage(`{"q":"Acme"}`)); !errors.Is(err, unreachable) {
		t.Errorf("an unreachable store answered %v, want its fault", err)
	}
}

// A grouped page says which types hold more, and says so with an empty list
// when every type fit, which a ranked page never carries.
func TestAGroupedPageNamesTheTypesWithMore(t *testing.T) {
	for _, more := range [][]string{{"deal"}, {}} {
		engine := &rankingProbe{page: RecordSearchPage{TypesWithMore: more}}
		raw, err := searchRecordsOver(nil, engine).Handle(searchReader(), json.RawMessage(`{"q":"Acme","per_type":2}`))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(raw), `"types_with_more":[`) {
			t.Errorf("a grouped page with %v holding more answered %s", more, raw)
		}
		if page := searchedPage(t, raw); page.TypesWithMore == nil || !slices.Equal(*page.TypesWithMore, more) {
			t.Errorf("types_with_more = %v, want %v", page.TypesWithMore, more)
		}
	}
}

func TestANearMatchIsReadBackAndANoteSaysItIsNoMatch(t *testing.T) {
	kept, gone := ids.NewV7(), ids.NewV7()
	provider := &queryProbeProvider{records: map[ids.UUID]datasource.Record{
		kept: recordAt(datasource.EntityContact, time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC), true),
	}}
	engine := &rankingProbe{page: RecordSearchPage{NearMatches: []RecordNearMatch{
		{Ref: datasource.EntityRef{Type: datasource.EntityContact, ID: kept}, Title: "Anna Kowalczyk"},
		{Ref: datasource.EntityRef{Type: datasource.EntityContact, ID: gone}, Title: "Gone Kowalczyk"},
	}}}
	raw, err := searchRecordsOver(provider, engine).Handle(searchReader(), json.RawMessage(`{"q":"kowalcyk"}`))
	if err != nil {
		t.Fatal(err)
	}
	page := searchedPage(t, raw)
	if !engine.asked.NearMatches {
		t.Error("search_records did not ask the engine for near matches")
	}
	if len(page.Records) != 0 || len(page.NearMatches) != 1 || page.NearMatches[0].ID != kept {
		t.Fatalf("the answer is %s, want only the readable near match", raw)
	}
	if page.Coverage != CoverageCompleteExact {
		t.Errorf("coverage = %q, want %q: the exact set is complete and empty", page.Coverage, CoverageCompleteExact)
	}
	if len(page.Notes) != 1 || page.Notes[0].Code != CodeNearMatchesOnly {
		t.Errorf("notes = %+v, want one %s note", page.Notes, CodeNearMatchesOnly)
	}
	if strings.Contains(string(raw), "Gone Kowalczyk") {
		t.Errorf("an unreadable near match leaked: %s", raw)
	}
}
