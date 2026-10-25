// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/geocode"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func berlinAddress(country string) *crmcontracts.Address {
	return &crmcontracts.Address{
		Line1: new("Invalidenstr. 1"), PostalCode: new("10115"), City: new("Berlin"), Country: new(country),
	}
}

// nominatimAnswering serves one Nominatim search answer and records each query
// it was asked.
func nominatimAnswering(t *testing.T, body string) (*geocode.Nominatim, func() []string) {
	t.Helper()
	var mu sync.Mutex
	var asked []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		asked = append(asked, r.URL.Query().Get("q"))
		mu.Unlock()
		if _, err := w.Write([]byte(body)); err != nil {
			t.Errorf("answering the geocode request: %v", err)
		}
	}))
	t.Cleanup(srv.Close)
	return geocode.NewNominatim(srv.URL, srv.Client()), func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), asked...)
	}
}

func runAddressCountryPass(t *testing.T, e *integration.Env, geocoder geocode.Client, at time.Time) {
	t.Helper()
	worker := newAddressCountryWorker(e.Pool, geocoder, func() time.Time { return at })
	if err := worker.Work(context.Background(), &river.Job[AddressCountryCheckArgs]{}); err != nil {
		t.Fatalf("running the address country check: %v", err)
	}
}

type countryCheckState struct {
	resolved  []string
	checkedAt *time.Time
}

func (s countryCheckState) String() string {
	resolved, checked := "none", "never"
	if s.resolved != nil {
		resolved = strings.Join(s.resolved, "+")
	}
	if s.checkedAt != nil {
		checked = s.checkedAt.Format(time.RFC3339)
	}
	return "resolved " + resolved + ", checked " + checked
}

func readCountryCheck(t *testing.T, e *integration.Env, table string, id ids.UUID) countryCheckState {
	t.Helper()
	var state countryCheckState
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT countries, checked_at FROM `+table+`_address_country WHERE `+table+`_id = $1`, id).
			Scan(&state.resolved, &state.checkedAt)
	})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("reading the country check of %s %s: %v", table, id, err)
	}
	return state
}

func createCompanyAt(t *testing.T, e *integration.Env, name string, address *crmcontracts.Address) ids.CompanyID {
	t.Helper()
	company, err := e.Contacts.CreateCompany(e.Admin(), contacts.CreateCompanyInput{
		DisplayName: name, Source: "manual", Address: address,
	})
	if err != nil {
		t.Fatalf("creating %s: %v", name, err)
	}
	return ids.From[ids.CompanyKind](ids.UUID(company.Id))
}

func createContactAt(t *testing.T, e *integration.Env, name string, address *crmcontracts.Address) ids.ContactID {
	t.Helper()
	contact, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{
		FullName: name, Source: "manual", Address: address,
	})
	if err != nil {
		t.Fatalf("creating %s: %v", name, err)
	}
	return ids.From[ids.ContactKind](ids.UUID(contact.Id))
}

func TestAStoredCountryThatDoesNotFitItsPostalCodeAndCityIsFlagged(t *testing.T) {
	e := integration.Setup(t)
	austrian := createCompanyAt(t, e, "Wrong Country GmbH", berlinAddress("AT"))
	german := createCompanyAt(t, e, "Right Country GmbH", berlinAddress("DE"))
	member := createContactAt(t, e, "Anna Weber", berlinAddress("AT"))
	nominatim, asked := nominatimAnswering(t, `[{"lat":"52.53","lon":"13.38","address":{"country_code":"de"}}]`)

	runAddressCountryPass(t, e, nominatim, clocktest.Now(t))

	if got := asked(); len(got) != 1 || got[0] != "10115 Berlin" {
		t.Fatalf("the provider was asked %q; want one question naming the postal code and city alone, the rest answered from the place cache", got)
	}
	company, err := e.Contacts.GetCompany(e.Admin(), austrian, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if company.AddressCountryMismatch == nil || company.AddressCountryMismatch.ResolvedCountry != "DE" {
		t.Errorf("a Berlin postal code filed under AT reads mismatch %+v; want resolved_country DE", company.AddressCountryMismatch)
	}
	if company.Address == nil || company.Address.Country == nil || *company.Address.Country != "AT" {
		t.Errorf("the check changed the stored country to %+v; it must only flag it", company.Address)
	}
	contact, err := e.Contacts.GetContact(e.Admin(), member, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if contact.AddressCountryMismatch == nil || contact.AddressCountryMismatch.ResolvedCountry != "DE" {
		t.Errorf("a contact with a Berlin postal code filed under AT reads mismatch %+v; want resolved_country DE", contact.AddressCountryMismatch)
	}
	matching, err := e.Contacts.GetCompany(e.Admin(), german, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if matching.AddressCountryMismatch != nil {
		t.Errorf("a Berlin postal code filed under DE is flagged %+v; a matching country shows no flag", matching.AddressCountryMismatch)
	}
	if state := readCountryCheck(t, e, "company", german.UUID); state.checkedAt == nil {
		t.Error("the matching company was not stamped as checked, so every pass would ask about it again")
	}
}

func TestEditingPostalCodeCityOrCountryClearsTheCountryCheck(t *testing.T) {
	edits := map[string]func(*crmcontracts.Address){
		"postal code": func(a *crmcontracts.Address) { a.PostalCode = new("80331") },
		"city":        func(a *crmcontracts.Address) { a.City = new("München") },
		"country":     func(a *crmcontracts.Address) { a.Country = new("DE") },
	}
	for column, edit := range edits {
		t.Run("company "+column, func(t *testing.T) {
			e := integration.Setup(t)
			id := createCompanyAt(t, e, "Moving GmbH", berlinAddress("AT"))
			stampCountryCheck(t, e, contacts.CountryCheckedCompany, id.UUID)
			address := berlinAddress("AT")
			edit(address)
			if _, err := e.Contacts.UpdateCompany(e.Admin(), id, contacts.UpdateCompanyInput{Address: address}); err != nil {
				t.Fatal(err)
			}
			if state := readCountryCheck(t, e, "company", id.UUID); state.resolved != nil || state.checkedAt != nil {
				t.Errorf("after editing the %s the company still holds check %v; the flag would speak for the old address", column, state)
			}
		})
		t.Run("contact "+column, func(t *testing.T) {
			e := integration.Setup(t)
			id := createContactAt(t, e, "Moving Contact", berlinAddress("AT"))
			stampCountryCheck(t, e, contacts.CountryCheckedContact, id.UUID)
			address := berlinAddress("AT")
			edit(address)
			if _, err := e.Contacts.UpdateContact(e.Admin(), id, contacts.UpdateContactInput{Address: address}); err != nil {
				t.Fatal(err)
			}
			if state := readCountryCheck(t, e, "contact", id.UUID); state.resolved != nil || state.checkedAt != nil {
				t.Errorf("after editing the %s the contact still holds check %v; the flag would speak for the old address", column, state)
			}
		})
	}
}

func TestEditingTheStreetKeepsTheCountryCheck(t *testing.T) {
	e := integration.Setup(t)
	id := createCompanyAt(t, e, "Same Town GmbH", berlinAddress("AT"))
	stampCountryCheck(t, e, contacts.CountryCheckedCompany, id.UUID)
	address := berlinAddress("AT")
	address.Line1 = new("Chausseestr. 5")
	if _, err := e.Contacts.UpdateCompany(e.Admin(), id, contacts.UpdateCompanyInput{Address: address}); err != nil {
		t.Fatal(err)
	}
	if state := readCountryCheck(t, e, "company", id.UUID); !slices.Equal(state.resolved, []string{"DE"}) {
		t.Errorf("a street edit cleared the check to %v; the postal code and city it answered for did not change", state)
	}
}

// stampCountryCheck records DE for one record through the sweep's own writer.
func stampCountryCheck(t *testing.T, e *integration.Env, record contacts.CountryCheckedRecord, id ids.UUID) {
	t.Helper()
	due, err := e.Contacts.ListAddressCountryChecksDue(e.Admin(), record, 10)
	if err != nil {
		t.Fatal(err)
	}
	for _, check := range due {
		if check.ID != id {
			continue
		}
		written, err := e.Contacts.WithClock(func() time.Time { return clocktest.Now(t) }).
			RecordAddressCountries(e.Admin(), check, []string{"DE"})
		if err != nil || !written {
			t.Fatalf("stamping the country check: written=%v err=%v", written, err)
		}
		return
	}
	t.Fatalf("%s %s is not due a country check", record, id)
}

// countriesFunc is a geocode.Client the test drives directly, for cases about
// the pass's own budget and races rather than the provider's wire format.
type countriesFunc func(ctx context.Context, query string) ([]string, error)

func (f countriesFunc) Resolve(context.Context, string) (geocode.Point, bool, error) {
	return geocode.Point{}, false, errors.New("the address country check never asks for a point")
}

func (f countriesFunc) Countries(ctx context.Context, query string) ([]string, error) {
	return f(ctx, query)
}

func TestACountryCheckLosesToAnEditMadeWhileItAsked(t *testing.T) {
	e := integration.Setup(t)
	id := createCompanyAt(t, e, "Racing GmbH", berlinAddress("AT"))
	editWhileAsking := countriesFunc(func(context.Context, string) ([]string, error) {
		address := berlinAddress("AT")
		address.City = new("Potsdam")
		if _, err := e.Contacts.UpdateCompany(e.Admin(), id, contacts.UpdateCompanyInput{Address: address}); err != nil {
			t.Errorf("editing the city while the provider is asked: %v", err)
		}
		return []string{"DE"}, nil
	})

	runAddressCountryPass(t, e, editWhileAsking, clocktest.Now(t))

	if state := readCountryCheck(t, e, "company", id.UUID); state.resolved != nil || state.checkedAt != nil {
		t.Errorf("the check for 10115 Berlin landed on a row now in Potsdam: %v", state)
	}
}

func TestOnePassAsksTheProviderAtMostItsBudget(t *testing.T) {
	e := integration.Setup(t)
	cities := []string{"Hamburg", "Köln", "Bremen", "Dresden", "Leipzig", "Kiel"}
	for _, city := range cities {
		createCompanyAt(t, e, city+" GmbH", &crmcontracts.Address{
			PostalCode: new("12345"), City: new(city), Country: new("DE"),
		})
	}
	var asked []string
	counting := countriesFunc(func(_ context.Context, query string) ([]string, error) {
		asked = append(asked, query)
		return []string{"DE"}, nil
	})

	runAddressCountryPass(t, e, counting, clocktest.Now(t))
	if len(asked) != addressCountryLookupsPerRun {
		t.Fatalf("one pass asked the provider %d times; want its budget of %d", len(asked), addressCountryLookupsPerRun)
	}
	runAddressCountryPass(t, e, counting, clocktest.Now(t))
	if len(asked) != len(cities) {
		t.Errorf("two passes asked %d times over %d unchecked companies; the second pass should take the rest and nothing twice", len(asked), len(cities))
	}
}

func TestAPostalCodeNamingTwoCountriesFlagsOnlyACountryItNamesNeither(t *testing.T) {
	e := integration.Setup(t)
	border := func(country string) *crmcontracts.Address {
		return &crmcontracts.Address{PostalCode: new("87567"), City: new("Riezlern"), Country: new(country)}
	}
	austrian := createCompanyAt(t, e, "Grenzland GmbH", border("AT"))
	swiss := createCompanyAt(t, e, "Falsch Abgelegt AG", border("CH"))
	nominatim, asked := nominatimAnswering(t,
		`[{"lat":"47.35","lon":"10.18","address":{"country_code":"de"}},{"lat":"47.36","lon":"10.18","address":{"country_code":"at"}}]`)

	runAddressCountryPass(t, e, nominatim, clocktest.Now(t))

	if got := asked(); len(got) != 1 {
		t.Fatalf("the provider was asked %q; want one question, the second company answered from the country cache", got)
	}
	kept, err := e.Contacts.GetCompany(e.Admin(), austrian, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if kept.AddressCountryMismatch != nil {
		t.Errorf("AT is one of the two countries 87567 Riezlern names, and it is flagged %+v", kept.AddressCountryMismatch)
	}
	flagged, err := e.Contacts.GetCompany(e.Admin(), swiss, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if flagged.AddressCountryMismatch == nil || flagged.AddressCountryMismatch.ResolvedCountry != "DE" {
		t.Errorf("CH is neither country 87567 Riezlern names, and it reads mismatch %+v; want resolved_country DE", flagged.AddressCountryMismatch)
	}
}

type recordStamp struct {
	version   int64
	updatedAt time.Time
}

func readRecordStamp(t *testing.T, e *integration.Env, table string, id ids.UUID) recordStamp {
	t.Helper()
	var stamp recordStamp
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT version, updated_at FROM `+table+` WHERE id = $1`, id).Scan(&stamp.version, &stamp.updatedAt)
	}); err != nil {
		t.Fatalf("reading the version of %s %s: %v", table, id, err)
	}
	return stamp
}

func TestACountryCheckWriteLeavesTheRecordVersionAlone(t *testing.T) {
	e := integration.Setup(t)
	company := createCompanyAt(t, e, "Open Form GmbH", berlinAddress("AT"))
	member := createContactAt(t, e, "Open Form Contact", berlinAddress("AT"))
	companyBefore := readRecordStamp(t, e, "company", company.UUID)
	contactBefore := readRecordStamp(t, e, "contact", member.UUID)

	runAddressCountryPass(t, e, countriesFunc(func(context.Context, string) ([]string, error) {
		return []string{"DE"}, nil
	}), clocktest.Now(t))

	if state := readCountryCheck(t, e, "company", company.UUID); state.checkedAt == nil {
		t.Fatal("the pass did not check the company, so this test proves nothing about its write")
	}
	if got := readRecordStamp(t, e, "company", company.UUID); got != companyBefore {
		t.Errorf("the check moved the company from %+v to %+v; an open edit form would fail its If-Match", companyBefore, got)
	}
	if got := readRecordStamp(t, e, "contact", member.UUID); got != contactBefore {
		t.Errorf("the check moved the contact from %+v to %+v; an open edit form would fail its If-Match", contactBefore, got)
	}
	if _, err := e.Contacts.UpdateCompany(e.Admin(), company, contacts.UpdateCompanyInput{
		Description: new("Edited in a form opened before the check"), IfVersion: &companyBefore.version,
	}); err != nil {
		t.Fatalf("a company edit carrying the version read before the check was refused: %v", err)
	}
	if _, err := e.Contacts.UpdateContact(e.Admin(), member, contacts.UpdateContactInput{
		Title: new("Edited in a form opened before the check"), IfVersion: &contactBefore.version,
	}); err != nil {
		t.Fatalf("a contact edit carrying the version read before the check was refused: %v", err)
	}
	if got := readRecordStamp(t, e, "company", company.UUID); got.version != companyBefore.version+1 {
		t.Errorf("a real edit left the company at version %d; want %d", got.version, companyBefore.version+1)
	}
}

func TestACountryCheckOlderThanTheRefreshAgeIsAskedAgain(t *testing.T) {
	e := integration.Setup(t)
	createCompanyAt(t, e, "Old Evidence GmbH", berlinAddress("DE"))
	var asked int
	counting := countriesFunc(func(context.Context, string) ([]string, error) {
		asked++
		return []string{"DE"}, nil
	})
	checkedAt := clocktest.Now(t)

	runAddressCountryPass(t, e, counting, checkedAt)
	runAddressCountryPass(t, e, counting, checkedAt.Add(contacts.AddressCountryRefreshAge-time.Hour))
	if asked != 1 {
		t.Fatalf("the provider was asked %d times before the evidence aged out; want once", asked)
	}
	runAddressCountryPass(t, e, counting, checkedAt.Add(contacts.AddressCountryRefreshAge+time.Hour))
	if asked != 2 {
		t.Errorf("the provider was asked %d times once the check and its cached answer were past the refresh age; want twice", asked)
	}
}
