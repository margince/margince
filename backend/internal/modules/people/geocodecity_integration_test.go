// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package people

import (
	"math"
	"testing"

	crmcontracts "github.com/gradionhq/margince/backend/internal/contracts"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
)

// A caller naming a CITY gets a point, out of the companies already located in
// it.
//
// This is the shape a person actually sends. "My meeting was cancelled and I am
// in Cologne" names a city; it never names the street of a company they have
// not reached yet. The place cache is keyed on the full address that was
// geocoded, so that one form was the one form that missed — and a radius
// answered nothing while the coordinates to answer it sat in the same estate
// under a longer key.
func TestACityNameResolvesFromTheCompaniesLocatedInIt(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()

	located := []struct {
		name     string
		city     string
		lat, lon float64
	}{
		{"Rheinland Nord", "Cologne", 50.9500, 6.9600},
		{"Rheinland Süd", "Cologne", 50.9300, 6.9400},
		{"Elsewhere Gmbh", "Hamburg", 53.5511, 9.9937},
	}
	for _, row := range located {
		org, err := e.store.CreateOrganization(ctx, CreateOrganizationInput{
			DisplayName: row.name, Source: "manual",
			Address: &crmcontracts.Address{
				Line1: strPtr("Teststrasse 1"),
				City:  strPtr(row.city),
			},
		})
		if err != nil {
			t.Fatalf("seeding %q: %v", row.name, err)
		}
		orgID := ids.From[ids.OrganizationKind](ids.UUID(org.Id))
		addr, ok, err := e.store.AddressForGeocode(ctx, orgID)
		if err != nil || !ok {
			t.Fatalf("reading %q back for geocoding: %v (found %v)", row.name, err, ok)
		}
		if err := e.store.RecordGeocode(ctx, orgID, GeocodeOK,
			&row.lat, &row.lon, "fake", addr.InputHash); err != nil {
			t.Fatalf("locating %q: %v", row.name, err)
		}
	}

	place, ok, err := e.store.LookupCity(ctx, "Cologne")
	if err != nil {
		t.Fatalf("LookupCity: %v", err)
	}
	if !ok {
		t.Fatal("a city holding two located companies resolved to nothing, so a radius around it answers no rows")
	}
	// The centroid of the two Cologne rows, and NOT the Hamburg one: a city
	// lookup that averaged the whole estate would answer a point in no city at
	// all.
	if math.Abs(place.Lat-50.9400) > 0.0001 || math.Abs(place.Lon-6.9500) > 0.0001 {
		t.Errorf("Cologne resolved to %.4f,%.4f, want the centroid of its own companies (50.9400,6.9500)",
			place.Lat, place.Lon)
	}
	// The provider says what the point IS. A reader comparing two centres has
	// to tell a geocoded place from one averaged out of the estate's own rows.
	if place.Provider != ProviderLocatedCompanies {
		t.Errorf("provider = %q, want %q — a derived point must not read as a geocoded one",
			place.Provider, ProviderLocatedCompanies)
	}
}

// Case and surrounding space are one question; a name nobody is located under
// is not answered approximately.
func TestACityLookupFoldsCaseAndRefusesWhatItCannotAnswer(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()

	org, err := e.store.CreateOrganization(ctx, CreateOrganizationInput{
		DisplayName: "Folded Case Gmbh", Source: "manual",
		Address: &crmcontracts.Address{Line1: strPtr("Teststrasse 2"), City: strPtr("Düsseldorf")},
	})
	if err != nil {
		t.Fatalf("seeding: %v", err)
	}
	orgID := ids.From[ids.OrganizationKind](ids.UUID(org.Id))
	addr, ok, err := e.store.AddressForGeocode(ctx, orgID)
	if err != nil || !ok {
		t.Fatalf("reading back for geocoding: %v (found %v)", err, ok)
	}
	lat, lon := 51.2277, 6.7735
	if err := e.store.RecordGeocode(ctx, orgID, GeocodeOK, &lat, &lon, "fake", addr.InputHash); err != nil {
		t.Fatalf("locating: %v", err)
	}

	for _, spelling := range []string{"Düsseldorf", "düsseldorf", "  DÜSSELDORF  "} {
		if _, ok, err := e.store.LookupCity(ctx, spelling); err != nil {
			t.Fatalf("LookupCity(%q): %v", spelling, err)
		} else if !ok {
			t.Errorf("%q did not resolve; case and surrounding space are one question", spelling)
		}
	}

	// A city nothing is located in answers NOT FOUND, so the caller says
	// distance_ranking_unavailable rather than measuring from a made-up point.
	// An approximate centre is the quietly wrong answer this whole surface
	// exists to avoid.
	if _, ok, err := e.store.LookupCity(ctx, "Reykjavík"); err != nil {
		t.Fatalf("LookupCity on an unheld city: %v", err)
	} else if ok {
		t.Error("a city holding no located company answered a point; a radius would then measure from nowhere")
	}

	// An empty name is not a city.
	if _, ok, err := e.store.LookupCity(ctx, "   "); err != nil {
		t.Fatalf("LookupCity on blank: %v", err)
	} else if ok {
		t.Error("blank resolved to a point, so every company with no city would answer as one place")
	}
}
