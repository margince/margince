// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"net/http"
	"net/http/httptest"
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
	resolved  *string
	checkedAt *time.Time
}

func (s countryCheckState) String() string {
	resolved, checked := "none", "never"
	if s.resolved != nil {
		resolved = *s.resolved
	}
	if s.checkedAt != nil {
		checked = s.checkedAt.Format(time.RFC3339)
	}
	return "resolved " + resolved + ", checked " + checked
}

func readCountryCheck(t *testing.T, e *integration.Env, table string, id ids.UUID) countryCheckState {
	t.Helper()
	var state countryCheckState
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT address_country_resolved, address_country_checked_at FROM `+table+` WHERE id = $1`, id).
			Scan(&state.resolved, &state.checkedAt)
	}); err != nil {
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
	person := createContactAt(t, e, "Anna Weber", berlinAddress("AT"))
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
	contact, err := e.Contacts.GetContact(e.Admin(), person, storekit.LiveOnly)
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
			id := createContactAt(t, e, "Moving Person", berlinAddress("AT"))
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
	if state := readCountryCheck(t, e, "company", id.UUID); state.resolved == nil || *state.resolved != "DE" {
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
			RecordAddressCountry(e.Admin(), check, "DE", true)
		if err != nil || !written {
			t.Fatalf("stamping the country check: written=%v err=%v", written, err)
		}
		return
	}
	t.Fatalf("%s %s is not due a country check", record, id)
}

// geocoderFunc is a geocode.Client the test drives directly, for cases about
// the pass's own budget and races rather than the provider's wire format.
type geocoderFunc func(ctx context.Context, query string) (geocode.Point, bool, error)

func (f geocoderFunc) Resolve(ctx context.Context, query string) (geocode.Point, bool, error) {
	return f(ctx, query)
}

func TestACountryCheckLosesToAnEditMadeWhileItAsked(t *testing.T) {
	e := integration.Setup(t)
	id := createCompanyAt(t, e, "Racing GmbH", berlinAddress("AT"))
	editWhileAsking := geocoderFunc(func(context.Context, string) (geocode.Point, bool, error) {
		address := berlinAddress("AT")
		address.City = new("Potsdam")
		if _, err := e.Contacts.UpdateCompany(e.Admin(), id, contacts.UpdateCompanyInput{Address: address}); err != nil {
			t.Errorf("editing the city while the provider is asked: %v", err)
		}
		return geocode.Point{Lat: 52.53, Lon: 13.38, CountryCode: "DE"}, true, nil
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
	counting := geocoderFunc(func(_ context.Context, query string) (geocode.Point, bool, error) {
		asked = append(asked, query)
		return geocode.Point{Lat: 50, Lon: 10, CountryCode: "DE"}, true, nil
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
