// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Search's employer arm: with WithEmployees, a query naming a company also
// finds the contacts who currently work there, each carrying that company as
// WorksAt. Every gate the contact list, the company list and the employment
// edge take is taken here too, and a refusal drops the arm without failing the
// search.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/search"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestAContactIsFoundThroughTheCompanyTheyWorkAt(t *testing.T) {
	e := SetupSearch(t)
	werke := seedSearchCompany(t, e, "Quedlinburg Werke")
	anna := seedSearchContact(t, e, "Anna Schmidt")
	staffCompany(t, e, anna, werke, employmentShape{})

	page := searchWith(e.Admin(), t, e, search.Input{Query: "quedlinburg", WithEmployees: true})
	hit := hitFor(page, anna)
	if hit == nil {
		t.Fatalf("the employee of a matched company was not found: %+v", page.Hits)
	}
	if hit.WorksAt == nil || hit.WorksAt.CompanyID != werke || hit.WorksAt.CompanyName != "Quedlinburg Werke" {
		t.Errorf("the employee carries works_at %+v, want the matched company %s", hit.WorksAt, werke)
	}
	if hit.Score >= 0 {
		t.Errorf("a contact found through its employer scores %v, want below every own-text hit", hit.Score)
	}

	page = searchWith(e.Admin(), t, e, search.Input{Query: "quedlinburg"})
	if hitFor(page, anna) != nil {
		t.Error("the employee was found without WithEmployees — the agent and plan lanes must keep matching by own text alone")
	}
}

// A contact whose own name matches is the contact branch's, once and without
// an employer, and it precedes every contact found only through one.
func TestAContactMatchedByItsOwnTextAppearsOnceAndFirst(t *testing.T) {
	e := SetupSearch(t)
	werke := seedSearchCompany(t, e, "Quedlinburg Werke")
	meier := seedSearchContact(t, e, "Karl Quedlinburg")
	anna := seedSearchContact(t, e, "Anna Schmidt")
	staffCompany(t, e, meier, werke, employmentShape{})
	staffCompany(t, e, anna, werke, employmentShape{})

	page := searchWith(e.Admin(), t, e, search.Input{Query: "quedlinburg", Types: []string{"contact"}, WithEmployees: true})
	if len(page.Hits) != 2 || page.Hits[0].ID != meier || page.Hits[1].ID != anna {
		t.Fatalf("contacts came back as %+v, want the own-text match %s then the employee %s", page.Hits, meier, anna)
	}
	if page.Hits[0].WorksAt != nil {
		t.Errorf("a contact matched by its own name carries works_at %+v", page.Hits[0].WorksAt)
	}
}

// Two matched employers, and two current rows at one of them, still make one
// contact: the hit names the employer that matched best.
func TestAContactAtTwoMatchedCompaniesAppearsOnce(t *testing.T) {
	e := SetupSearch(t)
	better := seedSearchCompany(t, e, "Quedlinburg Quedlinburg Werke")
	weaker := seedSearchCompany(t, e, "Quedlinburg Logistik")
	anna := seedSearchContact(t, e, "Anna Schmidt")
	staffCompany(t, e, anna, weaker, employmentShape{})
	staffCompany(t, e, anna, better, employmentShape{})
	staffCompany(t, e, anna, better, employmentShape{endsInDays: daysFromNow(30)})

	page := searchWith(e.Admin(), t, e, search.Input{Query: "quedlinburg", WithEmployees: true})
	var seen int
	for _, hit := range page.Hits {
		if hit.ID == anna {
			seen++
		}
	}
	if seen != 1 {
		t.Fatalf("the employee appears %d times, want once: %+v", seen, page.Hits)
	}
	scores := map[ids.UUID]float64{}
	for _, hit := range page.Hits {
		if hit.Type == "company" {
			scores[hit.ID] = hit.Score
		}
	}
	if scores[better] <= scores[weaker] {
		t.Fatalf("the fixture's ranks are %v and %v; it no longer tells the two employers apart", scores[better], scores[weaker])
	}
	hit := hitFor(page, anna)
	if hit == nil {
		t.Fatalf("the employee is not on the page: %+v", page.Hits)
	}
	if hit.WorksAt == nil || hit.WorksAt.CompanyID != better {
		t.Errorf("the employee carries works_at %+v, want the better-matching %s", hit.WorksAt, better)
	}
}

// Currency is employment.IsCurrentSQL's: a notice period still works there, a
// past departure or a former status does not, and only employment counts.
func TestOnlyACurrentEmploymentFindsAContact(t *testing.T) {
	e := SetupSearch(t)
	werke := seedSearchCompany(t, e, "Quedlinburg Werke")
	notice := seedSearchContact(t, e, "Nina Notice")
	former := seedSearchContact(t, e, "Fritz Former")
	departed := seedSearchContact(t, e, "Dora Departed")
	billing := seedSearchContact(t, e, "Bert Billing")
	archived := seedSearchContact(t, e, "Arne Archived")
	staffCompany(t, e, notice, werke, employmentShape{endsInDays: daysFromNow(30)})
	formerStatus := "former"
	staffCompany(t, e, former, werke, employmentShape{status: &formerStatus})
	staffCompany(t, e, departed, werke, employmentShape{endsInDays: daysFromNow(-30)})
	staffCompany(t, e, archived, werke, employmentShape{archived: true})
	e.SeedID(t, `INSERT INTO relationship (id, kind, contact_id, company_id, role, source, captured_by)
		VALUES ($1, 'billing_contact', $2, $3, 'recipient', 'manual', 'human:x')`, billing, werke)

	page := searchWith(e.Admin(), t, e, search.Input{Query: "quedlinburg", WithEmployees: true})
	if hitFor(page, notice) == nil {
		t.Error("a contact serving notice was not found — they still work there")
	}
	for name, contact := range map[string]ids.UUID{
		"a former employee": former, "a departed employee": departed,
		"an invoice recipient": billing, "an archived employment": archived,
	} {
		if hitFor(page, contact) != nil {
			t.Errorf("%s was found through the company", name)
		}
	}
}

// The installation's own company is not an account to find, and its staff are
// its employees: typing its name must not list every colleague.
func TestTheOwnCompanyFindsNoOne(t *testing.T) {
	e := SetupSearch(t)
	own := e.SeedID(t, `INSERT INTO company (id, display_name, is_anchor, source, captured_by)
		VALUES ($1, 'Quedlinburg Consulting', true, 'manual', 'human:x')`)
	colleague := seedSearchContact(t, e, "Clara Colleague")
	staffCompany(t, e, colleague, own, employmentShape{})

	page := searchWith(e.Admin(), t, e, search.Input{Query: "quedlinburg", WithEmployees: true})
	if len(page.Hits) != 0 {
		t.Fatalf("searching the own company's name found %+v, want nothing", page.Hits)
	}
}

// Each of the three gates drops the arm on its own, and none of them fails the
// search.
func TestTheEmployerArmNeedsEveryGrant(t *testing.T) {
	e := SetupSearch(t)
	werke := seedSearchCompany(t, e, "Quedlinburg Werke")
	anna := seedSearchContact(t, e, "Anna Schmidt")
	staffCompany(t, e, anna, werke, employmentShape{})

	for _, missing := range []string{objRelationship, objCompany, objContact} {
		grants := searchReadGrants()
		delete(grants, missing)
		reader := searchSeat(e, nil, principal.RowScopeAll, grants)
		page := searchWith(reader, t, e, search.Input{Query: "quedlinburg", WithEmployees: true})
		if hitFor(page, anna) != nil {
			t.Errorf("a seat without %s read found the employee through the company", missing)
		}
	}
}

// The arm reads under both row scopes: a private contact stays hidden behind a
// public employer, and a private company seeds no one.
func TestTheEmployerArmKeepsBothRowScopes(t *testing.T) {
	e := SetupSearch(t)
	public := seedSearchCompany(t, e, "Quedlinburg Werke")
	private := e.SeedID(t, `INSERT INTO company (id, display_name, owner_id, visibility, source, captured_by)
		VALUES ($1, 'Quedlinburg Privat AG', $2, 'owner', 'manual', 'human:x')`, e.Rep3)
	hidden := e.SeedID(t, `INSERT INTO contact (id, full_name, owner_id, visibility, source, captured_by)
		VALUES ($1, 'Hanna Hidden', $2, 'owner', 'manual', 'human:x')`, e.Rep3)
	seen := seedSearchContact(t, e, "Sven Seen")
	atPrivate := seedSearchContact(t, e, "Paula Private")
	staffCompany(t, e, hidden, public, employmentShape{})
	staffCompany(t, e, seen, public, employmentShape{})
	staffCompany(t, e, atPrivate, private, employmentShape{})

	page := searchWith(e.AsTeamRep(e.Rep1, e.Team1), t, e, search.Input{Query: "quedlinburg", WithEmployees: true})
	if hitFor(page, seen) == nil {
		t.Fatalf("the visible employee was not found, so this proves nothing: %+v", page.Hits)
	}
	if hitFor(page, hidden) != nil {
		t.Error("a contact captured privately by another rep was found through its employer")
	}
	if hitFor(page, atPrivate) != nil {
		t.Error("a company private to another rep seeded its employee")
	}
}

// An operator query is composed, not typed: `-zzz` matches nearly every
// company, so it reaches no one through one.
func TestAnOperatorQueryFindsNoOneThroughAnEmployer(t *testing.T) {
	e := SetupSearch(t)
	werke := seedSearchCompany(t, e, "Quedlinburg Werke")
	anna := seedSearchContact(t, e, "Anna Schmidt")
	staffCompany(t, e, anna, werke, employmentShape{})

	for _, query := range []string{"-zzz", "quedlinburg or bremen", `"quedlinburg werke"`} {
		page := searchWith(e.Admin(), t, e, search.Input{Query: query, WithEmployees: true})
		for _, hit := range page.Hits {
			if hit.WorksAt != nil {
				t.Errorf("%q found %s through an employer", query, hit.ID)
			}
		}
	}
	page := searchWith(e.Admin(), t, e, search.Input{Query: "quedlinburg or bremen", WithEmployees: true})
	if hitFor(page, anna) != nil {
		t.Error("an operator query found the employee")
	}
}

// The grouped cap counts contacts, own-text matches first, and says the type
// holds more exactly when more contacts matched than the page carries.
func TestAGroupedPageCapsContactsFoundThroughEmployers(t *testing.T) {
	e := SetupSearch(t)
	werke := seedSearchCompany(t, e, "Quedlinburg Werke")
	own := seedSearchContact(t, e, "Karl Quedlinburg")
	viaFirst := seedSearchContact(t, e, "Anna Schmidt")
	staffCompany(t, e, viaFirst, werke, employmentShape{})
	two := 2

	page := searchWith(e.Admin(), t, e, search.Input{Query: "quedlinburg", PerType: &two, WithEmployees: true})
	if got := contactIDs(page); len(got) != 2 || got[0] != own || got[1] != viaFirst {
		t.Fatalf("grouped contacts are %v, want %s then %s", got, own, viaFirst)
	}
	if len(page.TypesWithMore) != 0 {
		t.Errorf("types_with_more = %v with every contact on the page", page.TypesWithMore)
	}

	for _, name := range []string{"Bruno Becker", "Carla Clausen"} {
		staffCompany(t, e, seedSearchContact(t, e, name), werke, employmentShape{})
	}
	page = searchWith(e.Admin(), t, e, search.Input{Query: "quedlinburg", PerType: &two, WithEmployees: true})
	if got := contactIDs(page); len(got) != 2 || got[0] != own {
		t.Fatalf("grouped contacts are %v, want two led by the own-text match %s", got, own)
	}
	if hit := hitFor(page, own); hit == nil || hit.WorksAt != nil {
		t.Fatalf("the own-text match is %+v, want it on the page carrying no employer", hit)
	}
	if len(page.TypesWithMore) != 1 || page.TypesWithMore[0] != "contact" {
		t.Errorf("types_with_more = %v, want [contact] with four contacts matched and two shown", page.TypesWithMore)
	}
}

func TestARankedWalkWithEmployeesReturnsEachContactOnce(t *testing.T) {
	e := SetupSearch(t)
	werke := seedSearchCompany(t, e, "Quedlinburg Werke")
	own := map[ids.UUID]bool{}
	for _, name := range []string{"Karl Quedlinburg", "Lena Quedlinburg", "Max Quedlinburg"} {
		contact := seedSearchContact(t, e, name)
		staffCompany(t, e, contact, werke, employmentShape{})
		own[contact] = true
	}
	want := len(own)
	for _, name := range []string{"Anna Schmidt", "Bruno Becker", "Carla Clausen", "Dirk Dietz"} {
		staffCompany(t, e, seedSearchContact(t, e, name), werke, employmentShape{})
		want++
	}

	seen := map[ids.UUID]bool{}
	var order []ids.UUID
	cursor := ""
	for pages := 0; pages < 10; pages++ {
		in := search.Input{Query: "quedlinburg", Types: []string{"contact"}, Limit: 2, Cursor: cursor, WithEmployees: true}
		page := searchWith(e.Admin(), t, e, in)
		for _, hit := range page.Hits {
			if seen[hit.ID] {
				t.Fatalf("contact %s came back twice across the walk", hit.ID)
			}
			seen[hit.ID] = true
			order = append(order, hit.ID)
		}
		if !page.HasMore {
			break
		}
		cursor = page.NextCursor
	}
	if len(seen) != want {
		t.Fatalf("the walk returned %d contacts, want %d", len(seen), want)
	}
	for at, contact := range order {
		if own[contact] != (at < len(own)) {
			t.Fatalf("walk order %v does not put the own-text matches first", order)
		}
	}
}

// A company hit carries the URL its record carries, through the reader compose
// wires, and nothing when the company wears no logo.
func TestACompanyHitCarriesItsLogo(t *testing.T) {
	e := SetupSearch(t)
	key := "logos/quedlinburg-werke.png"
	branded := e.SeedID(t, `INSERT INTO company (id, display_name, logo_object_key, source, captured_by)
		VALUES ($1, 'Quedlinburg Werke', $2, 'manual', 'human:x')`, key)
	plain := seedSearchCompany(t, e, "Quedlinburg Logistik")

	store := search.NewStore(e.DB()).WithCompanyLogos(contacts.CompanyLogoURLsBatch)
	page, err := store.Search(e.Admin(), search.Input{Query: "quedlinburg", Types: []string{"company"}})
	if err != nil {
		t.Fatal(err)
	}
	logos := map[ids.UUID]*string{}
	for _, hit := range page.Hits {
		logos[hit.ID] = hit.LogoURL
	}
	if want := contacts.LogoURL(branded, &key, contacts.LogoWide); logos[branded] == nil || *logos[branded] != *want {
		t.Errorf("the branded company's hit carries logo %v, want %s", logos[branded], *want)
	}
	if got, ok := logos[plain]; !ok || got != nil {
		t.Errorf("the plain company's hit carries logo %v (present=%t), want a hit with none", got, ok)
	}
}

// The handler turns a decoded with_employees into the arm, and the wire carries
// the employer it found. Decoding the query is the generated wrapper's.
func TestTheSearchEndpointFindsEmployeesWhenAsked(t *testing.T) {
	e := SetupSearch(t)
	werke := seedSearchCompany(t, e, "Quedlinburg Werke")
	anna := seedSearchContact(t, e, "Anna Schmidt")
	staffCompany(t, e, anna, werke, employmentShape{})

	asked := true
	h := search.NewHandlers(e.DB(), nil, nil, nil, nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/search", nil).WithContext(e.Admin())
	h.Search(rec, req, crmcontracts.SearchParams{Q: "quedlinburg", WithEmployees: &asked})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var body crmcontracts.SearchResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	for _, result := range body.Data {
		if ids.UUID(result.Id) != anna {
			continue
		}
		if result.WorksAt == nil || result.WorksAt.CompanyName != "Quedlinburg Werke" {
			t.Fatalf("the employee's works_at is %+v, want Quedlinburg Werke", result.WorksAt)
		}
		return
	}
	t.Fatalf("the employee is not on the wire: %s", rec.Body.String())
}

func seedSearchCompany(t *testing.T, e *SearchEnv, name string) ids.UUID {
	t.Helper()
	return e.SeedID(t, `INSERT INTO company (id, display_name, source, captured_by) VALUES ($1, $2, 'manual', 'human:x')`, name)
}

func seedSearchContact(t *testing.T, e *SearchEnv, name string) ids.UUID {
	t.Helper()
	return e.SeedID(t, `INSERT INTO contact (id, full_name, source, captured_by) VALUES ($1, $2, 'manual', 'human:x')`, name)
}

// employmentShape shapes one seeded employment row; the zero value is an
// open-ended, current one.
type employmentShape struct {
	endsInDays *int
	status     *string
	archived   bool
}

// staffCompany records contact working at company, shaped as given.
func staffCompany(t *testing.T, e *SearchEnv, contact, company ids.UUID, shape employmentShape) {
	t.Helper()
	e.SeedID(t, `INSERT INTO relationship (id, kind, contact_id, company_id, ended_at, employment_status, archived_at, source, captured_by)
		VALUES ($1, 'employment', $2, $3, current_date + $4::int, $5, CASE WHEN $6 THEN now() END, 'manual', 'human:x')`,
		contact, company, shape.endsInDays, shape.status, shape.archived)
}

func daysFromNow(days int) *int { return &days }

func searchWith(ctx context.Context, t *testing.T, e *SearchEnv, in search.Input) search.Page {
	t.Helper()
	page, err := e.Store.Search(ctx, in)
	if err != nil {
		t.Fatalf("search %q: %v", in.Query, err)
	}
	return page
}

func contactIDs(page search.Page) []ids.UUID {
	var out []ids.UUID
	for _, hit := range page.Hits {
		if hit.Type == "contact" {
			out = append(out, hit.ID)
		}
	}
	return out
}

// The palette's own question, end to end: a company and its staff made the way
// the app makes them, then half the company's name typed. Every other case here
// seeds employment by statement; this one proves the writers the app uses
// produce rows the arm finds.
func TestThePaletteFindsTheStaffOfACompanyMadeThroughTheApp(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	var company struct {
		ID string `json:"id"`
	}
	if status := e.Call(t, "POST", "/v1/companies", AnyMap{"display_name": "Straight"}, nil, &company); status != 201 {
		t.Fatalf("create company → %d", status)
	}
	staff := map[string]bool{}
	for _, name := range []string{"Anna Becker", "Ben Fischer", "Clara Wolf"} {
		var contact contactRecord
		if status := e.Call(t, "POST", "/v1/contacts", AnyMap{"full_name": name, "source": "manual"}, nil, &contact); status != 201 {
			t.Fatalf("create contact %s → %d", name, status)
		}
		linkEdge(t, e, AnyMap{"kind": "employment", "contact_id": contact.ID, "company_id": company.ID, "source": "manual"})
		staff[contact.ID] = true
	}

	// The parameters palettesearch.ts sends; per_type is its PALETTE_PER_TYPE.
	asked := url.Values{"q": {"strai"}, "per_type": {"3"}, "with_employees": {"true"}}
	var page crmcontracts.SearchResponse
	if status := e.Call(t, "GET", "/v1/search?"+asked.Encode(), nil, nil, &page); status != 200 {
		t.Fatalf("search → %d", status)
	}
	for _, hit := range page.Data {
		if hit.Type == crmcontracts.SearchResultTypeContact && hit.WorksAt != nil && hit.WorksAt.CompanyId.String() == company.ID {
			delete(staff, hit.Id.String())
		}
	}
	if len(staff) != 0 {
		t.Fatalf("staff %v missing from the palette's page %+v", staff, page.Data)
	}
}
