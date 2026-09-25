// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Ranked cross-object search (B-EP05.15): relevance over the generated
// search_tsv columns, row-scope enforced per branch (a hit IS a read),
// archived rows invisible, stable ranked-keyset pagination, and the
// PERF-3 posture proven structurally — the plan must ride the GIN
// index, not a sequential scan.

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/search"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A role with NO contact grant gets no contact hits — search must not
// out-see the entity lists (object RBAC before row scope).
func TestSearchHonorsObjectRBAC(t *testing.T) {
	e := SetupSearch(t)
	e.SeedID(t, `INSERT INTO contact (id, full_name, source, captured_by) VALUES ($1, 'Rostock Contact', 'manual', 'human:x')`)
	e.SeedID(t, `INSERT INTO company (id, display_name, source, captured_by) VALUES ($1, 'Rostock Werft', 'manual', 'human:x')`)

	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	companyOnly := principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.Rep1.String(), UserID: e.Rep1,
		Permissions: principal.Permissions{
			Objects:  map[string]principal.ObjectGrant{"company": {Read: true}, "installation_settings": {Read: true}},
			RowScope: principal.RowScopeAll,
		},
	})
	page, err := e.Store.Search(companyOnly, search.Input{Query: "rostock"})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Hits) != 1 || page.Hits[0].Type != "company" {
		t.Fatalf("object RBAC leaked into search: %+v", page.Hits)
	}
	// Explicitly requesting only the denied type answers an empty page,
	// not an error — nothing to disclose.
	page, err = e.Store.Search(companyOnly, search.Input{Query: "rostock", Types: []string{"contact"}})
	if err != nil || len(page.Hits) != 0 {
		t.Fatalf("denied-type search → %v %+v, want an empty page", err, page.Hits)
	}
}

func TestSearchRanksAcrossObjectTypes(t *testing.T) {
	e := SetupSearch(t)
	e.SeedID(t, `INSERT INTO contact (id, full_name, source, captured_by) VALUES ($1, 'Heike Hamburg', 'manual', 'human:x')`)
	e.SeedID(t, `INSERT INTO company (id, display_name, source, captured_by) VALUES ($1, 'Hamburg Logistics GmbH', 'manual', 'human:x')`)
	e.SeedID(t, `INSERT INTO lead (id, company_name, email, source, captured_by) VALUES ($1, 'Hamburg Freight', 'lead@hamburg.test', 'manual', 'human:x')`)
	e.SeedID(t, `INSERT INTO activity (id, kind, subject, body, source, captured_by) VALUES ($1, 'note', 'Hamburg visit', 'Met the Hamburg team at the Hamburg office in Hamburg', 'manual', 'human:x')`)
	e.SeedID(t, `INSERT INTO contact (id, full_name, source, captured_by) VALUES ($1, 'Unrelated Munich', 'manual', 'human:x')`)

	page, err := e.Store.Search(e.Admin(), search.Input{Query: "hamburg"})
	if err != nil {
		t.Fatal(err)
	}
	types := map[string]bool{}
	for _, hit := range page.Hits {
		types[hit.Type] = true
		if hit.Score <= 0 {
			t.Fatalf("hit without a rank: %+v", hit)
		}
		if strings.Contains(hit.Title, "Munich") {
			t.Fatalf("non-matching row surfaced: %+v", hit)
		}
	}
	for _, want := range []string{"contact", "company", "lead", "activity"} {
		if !types[want] {
			t.Errorf("no %s hit in %+v", want, page.Hits)
		}
	}
	// The activity mentions the term four times — repetition ranks it
	// above single-mention rows.
	if page.Hits[0].Type != "activity" {
		t.Errorf("rank order ignores term frequency: top hit %+v", page.Hits[0])
	}
}

func TestSearchHitsCarryTheCallersRowScope(t *testing.T) {
	e := SetupSearch(t)
	// A contact is readable by every seat with the grant; a capture-private
	// one is readable by its owner alone, and a search hit IS a read.
	e.SeedID(t, `INSERT INTO contact (id, full_name, owner_id, visibility, source, captured_by) VALUES ($1, 'Scoped Bremen', $2, 'owner', 'manual', 'human:x')`, e.Rep3)
	e.SeedID(t, `INSERT INTO contact (id, full_name, source, captured_by) VALUES ($1, 'Shared Bremen', 'manual', 'human:x')`)

	// rep1 must not see rep3's private capture — but the ownerless row is
	// workspace-shared.
	page, err := e.Store.Search(e.AsTeamRep(e.Rep1, e.Team1), search.Input{Query: "bremen"})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Hits) != 1 || page.Hits[0].Title != "Shared Bremen" {
		t.Fatalf("row scope leaked into search: %+v", page.Hits)
	}
	// The captor sees both.
	page, err = e.Store.Search(e.AsTeamRep(e.Rep3, e.Team2), search.Input{Query: "bremen"})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Hits) != 2 {
		t.Fatalf("the captor sees %d, want 2", len(page.Hits))
	}
}

func TestSearchExcludesArchivedRows(t *testing.T) {
	e := SetupSearch(t)
	e.SeedID(t, `INSERT INTO contact (id, full_name, source, captured_by, archived_at) VALUES ($1, 'Archived Kiel', 'manual', 'human:x', now())`)
	page, err := e.Store.Search(e.Admin(), search.Input{Query: "kiel"})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Hits) != 0 {
		t.Fatalf("archived row surfaced: %+v", page.Hits)
	}
}

// Search is how contacts find ACCOUNTS, and the company running the CRM is not
// one to find (ADR-0082/A127). It stays an ordinary row, readable by id — what
// narrows is discovery.
func TestSearchExcludesTheOwnCompany(t *testing.T) {
	e := SetupSearch(t)
	e.SeedID(t, `INSERT INTO company (id, display_name, is_anchor, source, captured_by) VALUES ($1, 'Rostock Consulting GmbH', true, 'manual', 'human:x')`)
	customer := e.SeedID(t, `INSERT INTO company (id, display_name, source, captured_by) VALUES ($1, 'Rostock Freight AG', 'manual', 'human:x')`)

	page, err := e.Store.Search(e.Admin(), search.Input{Query: "rostock"})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Hits) != 1 || page.Hits[0].ID != customer {
		t.Fatalf("search returned %+v, want only the customer %s — the installation's own company is not an account to find", page.Hits, customer)
	}
}

// A partner is a PROPERTY of a company rather than something to search for, so
// the marker rides the company hit and is derived from the partner row's
// existence. Retired counts as none: a programme that has been wound up is not
// one to badge an account with.
func TestACompanyHitSaysWhetherTheAccountIsAPartner(t *testing.T) {
	e := SetupSearch(t)
	partner, none, retired := seedWismarAccounts(t, e)

	reader := searchSeat(e, nil, principal.RowScopeAll, searchGrantsWithPartner())
	page, err := partnerMarkingStore(e).Search(reader, search.Input{Query: "wismar", Types: []string{"company"}})
	if err != nil {
		t.Fatal(err)
	}

	marks := partnerMarks(t, page.Hits)
	for _, want := range []struct {
		name    string
		company ids.UUID
		marker  bool
	}{
		{"an account with a live programme", partner, true},
		{"an account with no programme", none, false},
		{"an account whose programme was retired", retired, false},
	} {
		got, ok := marks[want.company]
		if !ok {
			t.Errorf("%s produced no hit at all", want.name)
			continue
		}
		if got == nil {
			t.Errorf("%s is marked unknown, but this caller may read partner programmes", want.name)
			continue
		}
		if *got != want.marker {
			t.Errorf("%s is marked %t, want %t", want.name, *got, want.marker)
		}
	}
}

// A seat that may not read partner programmes keeps its company hits and gets
// no marker on them — null, never a false it was refused the basis for.
func TestThePartnerMarkerIsWithheldWithoutTheObjectGrant(t *testing.T) {
	e := SetupSearch(t)
	partner, _, _ := seedWismarAccounts(t, e)

	denied := searchSeat(e, nil, principal.RowScopeAll, searchGrantsWithoutPartner())
	page, err := partnerMarkingStore(e).Search(denied, search.Input{Query: "wismar", Types: []string{"company"}})
	if err != nil {
		t.Fatalf("a caller without the partner grant was refused their search: %v", err)
	}

	marks := partnerMarks(t, page.Hits)
	if len(marks) != 3 {
		t.Fatalf("the page carries %d company hit(s), want all three — the marker must not narrow the page", len(marks))
	}
	for company, marker := range marks {
		if marker != nil {
			t.Errorf("company %s is marked %t for a caller who may not read partner programmes", company, *marker)
		}
	}
	if _, ok := marks[partner]; !ok {
		t.Error("the partner account itself fell off the page")
	}
}

// The reader re-derives the company's own read gate instead of trusting
// whoever hands it the ids. A seat that may not see an account must not learn
// from a marker that it is a partner — not through search, and not by calling
// the reader directly with its id, which is what "exported" makes possible.
func TestThePartnerMarkerNeverOutseesTheCompanyRowScope(t *testing.T) {
	e := SetupSearch(t)
	// Captured privately by a rep on the other team, so rep1 may not read it.
	hidden := e.SeedID(t, `INSERT INTO company (id, display_name, owner_id, visibility, source, captured_by)
	                       VALUES ($1, 'Wismar Private AG', $2, 'owner', 'manual', 'human:x')`, e.Rep3)
	e.SeedID(t, `INSERT INTO partner (id, company_id, source, captured_by) VALUES ($1, $2, 'manual', 'human:x')`, hidden)
	rep := searchSeat(e, []ids.UUID{e.Team1}, principal.RowScopeTeam, searchGrantsWithPartner())

	page, err := partnerMarkingStore(e).Search(rep, search.Input{Query: "wismar", Types: []string{"company"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := partnerMarks(t, page.Hits)[hidden]; ok {
		t.Fatalf("search returned a company this seat may not read: %+v", page.Hits)
	}

	company := ids.From[ids.CompanyKind](hidden)
	var live map[ids.CompanyID]bool
	if err := e.DB().Tx(rep, func(tx pgx.Tx) error {
		var readErr error
		live, readErr = contacts.LivePartnerCompaniesBatch(rep, tx, []ids.CompanyID{company})
		return readErr
	}); err != nil {
		t.Fatal(err)
	}
	if live[company] {
		t.Error("the reader marked a company outside this seat's row scope as a partner")
	}
}

// seedWismarAccounts puts three accounts sharing one search term on the
// workspace: one with a live partner programme, one with none, and one whose
// programme was retired.
func seedWismarAccounts(t *testing.T, e *SearchEnv) (partner, none, retired ids.UUID) {
	t.Helper()
	company := func(name string) ids.UUID {
		return e.SeedID(t, `INSERT INTO company (id, display_name, source, captured_by) VALUES ($1, $2, 'manual', 'human:x')`, name)
	}
	partner, none, retired = company("Wismar Werft AG"), company("Wismar Freight GmbH"), company("Wismar Rigging KG")
	e.SeedID(t, `INSERT INTO partner (id, company_id, source, captured_by) VALUES ($1, $2, 'manual', 'human:x')`, partner)
	e.SeedID(t, `INSERT INTO partner (id, company_id, source, captured_by, archived_at) VALUES ($1, $2, 'manual', 'human:x', now())`, retired)
	return partner, none, retired
}

// partnerMarkingStore binds the real contacts reader, so what these suites
// assert is the seam compose wires rather than a stand-in for it.
func partnerMarkingStore(e *SearchEnv) *search.Store {
	return search.NewStore(e.DB()).WithPartnerMarks(contacts.LivePartnerCompaniesBatch)
}

// partnerMarks keys one page's markers by the company each hit names.
func partnerMarks(t *testing.T, hits []search.Hit) map[ids.UUID]*bool {
	t.Helper()
	marks := make(map[ids.UUID]*bool, len(hits))
	for _, hit := range hits {
		if hit.Type != "company" {
			t.Fatalf("a company-only search returned a %s hit", hit.Type)
		}
		marks[hit.ID] = hit.IsPartner
	}
	return marks
}

// searchGrantsWithPartner and searchGrantsWithoutPartner are the two sides of
// the axis the partner arms vary. Each states which it holds rather than
// inheriting it from searchObjects, so a grant added there later cannot turn
// the deny arm into a second copy of the allow arm without a word being said.
func searchGrantsWithPartner() map[string]principal.ObjectGrant {
	grants := searchReadGrants()
	grants[objPartner] = principal.ObjectGrant{Read: true}
	return grants
}

func searchGrantsWithoutPartner() map[string]principal.ObjectGrant {
	grants := searchReadGrants()
	delete(grants, objPartner)
	return grants
}

// objPartner gates a partner programme — a property of a company rather than
// one of the record types these suites search for, which is why the fixture's
// own vocabulary does not carry it.
const objPartner = "partner"

// searchSeat is rep1 holding exactly these grants at this row scope, in the
// teams given (nil for a seat no team bounds).
func searchSeat(e *SearchEnv, teams []ids.UUID, scope principal.RowScope, grants map[string]principal.ObjectGrant) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.Rep1.String(), UserID: e.Rep1,
		TeamIDs:     teams,
		Permissions: principal.Permissions{Objects: grants, RowScope: scope},
	})
}

func TestSearchRankedCursorWalksAllHitsOnce(t *testing.T) {
	e := SetupSearch(t)
	want := map[string]bool{}
	for i := 0; i < 5; i++ {
		id := e.SeedID(t, fmt.Sprintf(`INSERT INTO contact (id, full_name, source, captured_by) VALUES ($1, 'Dresden Contact %d', 'manual', 'human:x')`, i))
		want[id.String()] = false
	}
	got := 0
	cursor := ""
	for pages := 0; pages < 5; pages++ {
		page, err := e.Store.Search(e.Admin(), search.Input{Query: "dresden", Limit: 2, Cursor: cursor})
		if err != nil {
			t.Fatal(err)
		}
		for _, hit := range page.Hits {
			if seen, ok := want[hit.ID.String()]; !ok || seen {
				t.Fatalf("hit %s unknown or repeated", hit.ID)
			}
			want[hit.ID.String()] = true
			got++
		}
		if !page.HasMore {
			break
		}
		cursor = page.NextCursor
	}
	if got != 5 {
		t.Fatalf("cursor walk yielded %d of 5 hits", got)
	}
}

func TestSearchEmptyQueryIsAValidationError(t *testing.T) {
	e := SetupSearch(t)
	_, err := e.Store.Search(e.Admin(), search.Input{Query: "   "})
	var bad *search.BadQueryError
	if err == nil || !errors.As(err, &bad) || !strings.Contains(bad.Reason, "required") {
		t.Fatalf("empty query → %v, want BadQueryError", err)
	}
}

// The PERF-3 posture, proven structurally instead of by a flaky
// wall-clock or planner assertion: every table the search union reads
// must define a GIN index over search_tsv, so the FTS predicate CAN
// scale past a sequential scan. (Which plan the optimizer picks at a
// given cardinality is its business; the index existing is ours.)
func TestSearchEveryBranchHasAGinIndex(t *testing.T) {
	e := SetupSearch(t)
	// Derived from the branch table, never restated: a hand-kept copy of this
	// list had already fallen behind by two branches, and a census that reads a
	// smaller set than its subject reports PASS with nothing to notice.
	for _, table := range search.SearchedTables() {
		var exists bool
		err := e.Owner.QueryRow(context.Background(), `
			SELECT EXISTS (
			  SELECT 1 FROM pg_index i
			  JOIN pg_class idx ON idx.oid = i.indexrelid
			  JOIN pg_class tbl ON tbl.oid = i.indrelid
			  JOIN pg_am am ON am.oid = idx.relam
			  JOIN pg_attribute a ON a.attrelid = tbl.oid AND a.attnum = ANY(i.indkey)
			  WHERE tbl.relname = $1 AND am.amname = 'gin' AND a.attname = 'search_tsv')`,
			table).Scan(&exists)
		if err != nil {
			t.Fatal(err)
		}
		if !exists {
			t.Errorf("table %s is searched but has no GIN index over search_tsv", table)
		}
	}
}

// The catalog branches (product, offer_template). Both are workspaceWide: the
// price list and the offer layouts carry no owner, so a seat that may read them
// reads all of them — which makes the OBJECT grant the whole of the gate, and
// the one thing worth proving twice.
func TestSearchFindsTheCatalogByNameAndSku(t *testing.T) {
	e := SetupSearch(t)
	e.SeedID(t, `INSERT INTO product (id, name, sku, description, unit_price_minor, currency, source, captured_by)
	             VALUES ($1, 'Kärcher floor scrubber', 'KAR-9910', 'Industrial wet cleaning unit', 129900, 'EUR', 'manual', 'human:x')`)
	e.SeedID(t, `INSERT INTO offer_template (id, name, layout) VALUES ($1, 'Kärcher rollout quote', '{}'::jsonb)`)

	ctx := e.Admin()
	// The name, unaccented: somebody typing an ASCII keyboard finds the umlaut.
	page, err := e.Store.Search(ctx, search.Input{Query: "karcher"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"product", "offer_template"} {
		if !hasType(page.Hits, want) {
			t.Errorf("searching a catalog name returned no %s hit: %+v", want, page.Hits)
		}
	}

	// The sku is an 'A' arm, not just an excerpt: a rep holding a printed offer
	// has the sku and not the name.
	page, err = e.Store.Search(ctx, search.Input{Query: "KAR-9910", Types: []string{"product"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Hits) != 1 {
		t.Fatalf("searching a sku → %+v, want the one product", page.Hits)
	}
	if page.Hits[0].Snippet != "KAR-9910" {
		t.Errorf("product excerpt is %q, want the sku that tells two variants apart", page.Hits[0].Snippet)
	}
}

// A discontinued product is still findable. It stands on last quarter's offers,
// and a rep looking one up is usually holding one of them — so `active` is not
// a discovery narrowing, while archived_at (which every branch carries) is.
func TestSearchFindsAnInactiveProductButNotAnArchivedOne(t *testing.T) {
	e := SetupSearch(t)
	e.SeedID(t, `INSERT INTO product (id, name, active, unit_price_minor, currency, source, captured_by)
	             VALUES ($1, 'Zwickau retired mount', false, 4900, 'EUR', 'manual', 'human:x')`)
	e.SeedID(t, `INSERT INTO product (id, name, archived_at, unit_price_minor, currency, source, captured_by)
	             VALUES ($1, 'Zwickau archived mount', now(), 4900, 'EUR', 'manual', 'human:x')`)

	page, err := e.Store.Search(e.Admin(), search.Input{Query: "zwickau", Types: []string{"product"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Hits) != 1 {
		t.Fatalf("searching a retired catalog → %+v, want the inactive row alone", page.Hits)
	}
	if page.Hits[0].Title != "Zwickau retired mount" {
		t.Errorf("found %q, want the inactive product and never the archived one", page.Hits[0].Title)
	}
}

// A seat with no catalog grant gets no catalog hits, and asking for the type by
// name is an empty page rather than a 403 — the same existence-hiding posture
// every other branch takes.
func TestCatalogHitsAreWithheldWithoutTheObjectGrant(t *testing.T) {
	e := SetupSearch(t)
	e.SeedID(t, `INSERT INTO product (id, name, unit_price_minor, currency, source, captured_by)
	             VALUES ($1, 'Ilmenau bracket', 2500, 'EUR', 'manual', 'human:x')`)

	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	noCatalog := principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.Rep1.String(), UserID: e.Rep1,
		Permissions: principal.Permissions{
			Objects:  map[string]principal.ObjectGrant{objInstallSettings: {Read: true}},
			RowScope: principal.RowScopeAll,
		},
	})
	page, err := e.Store.Search(noCatalog, search.Input{Query: "ilmenau"})
	if err != nil || len(page.Hits) != 0 {
		t.Fatalf("catalog search without the grant → %v %+v, want an empty page", err, page.Hits)
	}
	page, err = e.Store.Search(noCatalog, search.Input{Query: "ilmenau", Types: []string{"product"}})
	if err != nil || len(page.Hits) != 0 {
		t.Fatalf("denied catalog type → %v %+v, want an empty page and never a refusal", err, page.Hits)
	}
}

// hasType answers whether any hit carries the type, which is what the
// cross-type assertions above are actually asking.
func hasType(hits []search.Hit, want string) bool {
	for _, hit := range hits {
		if hit.Type == want {
			return true
		}
	}
	return false
}

// The ceiling compose arms on the search surface still serves an ordinary
// search.
//
// The other half of the bound — that a spent ceiling reads as an actionable
// "too broad" rather than a 5xx — is asserted in the search module's own
// rankingfault_test.go, where it is a pure function of an error and a context.
// It is NOT asserted by racing a live statement against a one-millisecond
// budget: that test times the machine it runs on, and it flaked exactly that
// way when it was written here.
//
// What this one holds is the direction a unit test cannot: a ceiling that
// refused ordinary work would satisfy every assertion about the fault and break
// the product.
func TestTheSearchCeilingStillServesAnOrdinarySearch(t *testing.T) {
	e := SetupSearch(t)
	e.SeedID(t, `INSERT INTO contact (id, full_name, source, captured_by) VALUES ($1, 'Rostock Contact', 'manual', 'human:x')`)

	status, body := callSearch(t, e, database.CallerPredicateBudget, "Rostock")

	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200 under the ceiling compose arms: %s", status, body)
	}
	if !strings.Contains(body, "Rostock Contact") {
		t.Errorf("the seeded contact did not come back — the ceiling is refusing work it should "+
			"serve: %s", body)
	}
}

// callSearch drives GET /search through the module's own handler, over a handle
// bounded the way compose bounds the one it wires (server.go).
func callSearch(t *testing.T, e *SearchEnv, budget time.Duration, q string) (int, string) {
	t.Helper()
	db := database.BindTo(e.Pool, ids.From[ids.WorkspaceKind](e.WS)).Bounded(budget)
	h := search.NewHandlers(db, nil, nil, nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/search?q="+q, nil).WithContext(searchAs(e))
	h.Search(rec, req, crmcontracts.SearchParams{Q: q})
	return rec.Code, rec.Body.String()
}

// searchAs is a reader who may see everything, so the assertions below are
// about the ceiling rather than about scope.
func searchAs(e *SearchEnv) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.Rep1.String(), UserID: e.Rep1,
		Permissions: principal.Permissions{
			Objects: map[string]principal.ObjectGrant{
				"contact": {Read: true}, "company": {Read: true},
				"installation_settings": {Read: true},
			},
			RowScope: principal.RowScopeAll,
		},
	})
}
