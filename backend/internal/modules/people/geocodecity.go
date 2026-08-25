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
	var lat, lon *float64
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
		return tx.QueryRow(ctx, `
			SELECT avg(geocode_lat), avg(geocode_lon), count(*)
			  FROM organization
			 WHERE archived_at IS NULL
			   AND geocode_lat IS NOT NULL
			   AND geocode_lon IS NOT NULL
			   AND lower(btrim(address_city)) = $1
			   AND `+scope, args...).
			Scan(&lat, &lon, &located)
	})
	if err != nil {
		return CachedPlace{}, false, fmt.Errorf("reading located companies for %q: %w", city, err)
	}
	if located == 0 || lat == nil || lon == nil {
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
