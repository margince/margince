// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// Resolving a bare CITY NAME to a point, out of the addresses this installation
// has already located.
//
// WHY THIS EXISTS. The place cache is keyed on the exact string that was
// geocoded, and what gets geocoded is a company's full address — "arnulfstraße
// 33, 40545, düsseldorf". So a caller asking for everything within 50km of
// "Düsseldorf" missed: the cache holds the street, never the city on its own.
// A radius is the one shape a "my meeting was cancelled, who is nearby" question
// can take, and it answered nothing while the coordinates to answer it sat in
// the same table under a longer key.
//
// NO GEOCODER IS CALLED HERE, and that is the constraint the whole design turns
// on. query_workspace is declared workspace-local, Scope.Egresses() is derived
// from that declaration rather than asserted, and a resolver that could reach
// the internet would make the declaration unenforceable by construction. So the
// answer has to come from rows this installation already holds.
//
// It does. A company located in a city carries a point IN that city, so the
// centroid of the located companies sharing a city name is a point in that city
// — good to within the spread of the addresses themselves, which for a radius
// measured in tens of kilometres is far below the error already accepted in
// "which office is this company actually at".

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// LookupCity answers a point for a city NAME, derived from the located
// organizations whose address names that city.
//
// The comparison folds case and surrounding space, so "cologne", "Cologne" and
// " COLOGNE " are one question. It deliberately does NOT fold "Köln" onto
// "Cologne": those are two strings a geocoder would resolve to one place, and
// deciding they are the same city is a translation this function has no
// business making. A caller asking for Köln gets the companies filed under
// Köln.
//
// A city holding no located company answers not-found rather than an
// approximate point, and the caller says so honestly — which is what
// distance_ranking_unavailable already means.
//
// ROW-SCOPED, unlike the two place-cache reads beside it. Those hold a public
// coordinate for a public place name and carry no subject a grant could be
// about; this one composes its answer FROM companies, so an unscoped version
// would tell a caller that somebody is located in a city they may see none of
// the records in. The scope means the centre is averaged only over companies
// this caller could have listed for themselves.
func (s *Store) LookupCity(ctx context.Context, city string) (CachedPlace, bool, error) {
	key := normalizePlaceQuery(city)
	if key == "" {
		return CachedPlace{}, false, nil
	}
	// The averages are read as POINTERS, because a city nothing is located in
	// makes avg() answer NULL rather than no row at all: an aggregate with no
	// GROUP BY always returns exactly one row, and scanning that NULL into a
	// float64 is an error rather than the not-found this function means. Found
	// by the test for a city nobody is in.
	var lat, lon, latSpread, lonSpread *float64
	var located int
	err := s.tx(ctx, func(tx pgx.Tx) error {
		args := []any{key}
		arg := func(v any) int { args = append(args, v); return len(args) }
		// ROW SCOPE, and it is load-bearing rather than ceremonial. This
		// answers a point derived from companies, so without the predicate a
		// caller who may see none of them still learns that SOMEBODY is located
		// in a city — an existence answer about records they were not granted,
		// one city name at a time. Scoped, the centroid is composed only from
		// the companies this caller could have listed themselves.
		scope, err := scopeOrAllRows(ctx, "organization", "", arg)
		if err != nil {
			return err
		}
		// geocode_status = 'ok' is REQUIRED, and it is the same predicate the
		// radius query itself applies (querygeo.go). A row keeps its old point
		// when its address changes and is stamped `stale` — "not queryable,
		// deliberately" in geocode.go's own words — so averaging one in would
		// measure this city from where a company USED to be. Coordinates that
		// are merely non-NULL are not coordinates that are true.
		//
		// The SPREAD comes back with the centre, from the same scan: asking a
		// second time would read a different row set under a concurrent write,
		// and the check is only meaningful against the rows that produced this
		// average.
		return tx.QueryRow(ctx, `
			SELECT avg(geocode_lat), avg(geocode_lon),
			       max(geocode_lat) - min(geocode_lat),
			       max(geocode_lon) - min(geocode_lon),
			       count(*)
			  FROM organization
			 WHERE archived_at IS NULL
			   AND geocode_status = 'ok'
			   AND geocode_lat IS NOT NULL
			   AND geocode_lon IS NOT NULL
			   AND lower(btrim(address_city)) = $1
			   AND `+scope, args...).
			Scan(&lat, &lon, &latSpread, &lonSpread, &located)
	})
	if err != nil {
		return CachedPlace{}, false, fmt.Errorf("reading located companies for %q: %w", city, err)
	}
	if located == 0 || lat == nil || lon == nil {
		return CachedPlace{}, false, nil
	}
	// A SPREAD TOO WIDE TO BE ONE CITY answers not-found rather than its own
	// midpoint.
	//
	// Two things make a city name cover more than one place. Springfield is in
	// dozens of states and Frankfurt is in two German states, and this lane
	// compares the city text alone — it has no region or country to
	// disambiguate with. And a longitude average is plain wrong across the
	// antimeridian: 179 and -179 average to 0, which is the Gulf of Guinea.
	//
	// Both produce the same artefact — a centre far from every company that
	// voted for it — and one check catches both, because a real city's
	// companies sit within hundredths of a degree of each other.
	//
	// Refusing is the honest answer. The caller says
	// distance_ranking_unavailable and asks by city name instead, rather than
	// measuring 50 km around a point no company is near.
	if latSpread != nil && *latSpread > maxCitySpreadDegrees {
		return CachedPlace{}, false, nil
	}
	if lonSpread != nil && *lonSpread > maxCitySpreadDegrees {
		return CachedPlace{}, false, nil
	}
	place := CachedPlace{Lat: *lat, Lon: *lon}
	// The provider is named for what this point IS. A reader comparing two
	// centres has to be able to tell a geocoded place from one averaged out of
	// the estate's own rows, and "nominatim" would say the wrong thing.
	place.Provider = ProviderLocatedCompanies
	return place, true, nil
}

// ProviderLocatedCompanies marks a point derived from this installation's own
// located companies rather than fetched from a geocoding service.
const ProviderLocatedCompanies = "located_companies"

// maxCitySpreadDegrees is how far apart the companies sharing a city name may
// sit before their midpoint stops being a place.
//
// ONE DEGREE — roughly 111 km of latitude, less of longitude at European
// latitudes. An order of magnitude wider than any real city, and an order of
// magnitude narrower than the artefacts it refuses. Measured on a real
// installation, the widest genuine spread was 0.14° (Berlin, München), about
// 12 km; two Frankfurts sit 5.9° apart, and a longitude average taken across
// the antimeridian lands the midpoint a hemisphere from every company that
// voted for it.
//
// A midpoint that fails this is not returned at all. Answering it would mean
// measuring 50 km around a point no company is near — a wrong answer wearing a
// working answer's clothes, which is the failure this surface exists to refuse.
const maxCitySpreadDegrees = 1.0
