// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package geocode

import (
	"fmt"
	"math"
)

// Columns is the coordinate pair a radius predicate measures from, plus the
// status column that says whether the pair means anything.
type Columns struct {
	Lat, Lon, Status string
}

// ResolvedStatus is the one status a radius query may read, and the value
// contacts.GeocodeOK writes.
const ResolvedStatus = "ok"

// locatable maps a record type to the columns that make it locatable.
//
// Only `company`: a company has an address that means a place on the earth. A
// contact's address is where somebody lives, which this product does not
// geocode.
var locatable = map[string]Columns{
	"company": {Lat: "geocode_lat", Lon: "geocode_lon", Status: "geocode_status"},
}

// ColumnsFor returns the coordinate columns of a record type, and false for a
// type that is not somewhere.
func ColumnsFor(target string) (Columns, bool) {
	cols, ok := locatable[target]
	return cols, ok
}

// earthRadiusKM is the mean radius, which is what a haversine over a sphere
// takes. The earth is not a sphere and this is not a survey: over the
// distances a sales team asks about, the error is well under the error in
// "which office is this company actually at".
const earthRadiusKM = 6371.0

// boundingBox is the cheap pre-filter a radius query runs before any
// trigonometry.
//
// It exists so the index does the work. The haversine below is exact and
// unindexable — Postgres cannot use a btree on (lat, lon) to evaluate it — so
// the box narrows the candidate rows to something small enough for the exact
// distance to be cheap. The box is deliberately WIDER than the circle: it must
// never exclude a row the circle would have included, and including a few
// extra that the haversine then rejects costs nothing but a comparison.
func boundingBox(center Point, radiusKM float64) (minLat, maxLat, minLon, maxLon float64) {
	latDelta := radiusKM / earthRadiusKM * 180 / math.Pi
	minLat, maxLat = center.Lat-latDelta, center.Lat+latDelta

	// THE COSINE IS TAKEN AT THE WIDEST LATITUDE THE CIRCLE REACHES, not at
	// the center. Longitude degrees narrow toward the poles, so a circle
	// reaching further from the equator than its center bulges further east
	// and west than the center's own cosine predicts — and a box computed from
	// the center would cut inside the circle at those latitudes, silently
	// dropping companies that ARE within the radius. The first version of this
	// did exactly that; the test walking the circle's edge caught it.
	//
	// The widest latitude is whichever of the box's two edges sits further
	// from the equator, since |cos| shrinks as |latitude| grows.
	widest := math.Max(math.Abs(minLat), math.Abs(maxLat))
	// At the poles cos() approaches zero and the box would widen without
	// bound, so it is clamped to the whole range — a radius query near a pole
	// falls back to the haversine alone, which is correct and rare.
	cos := math.Cos(math.Min(widest, 90) * math.Pi / 180)
	lonDelta := 180.0
	if cos > 0.01 {
		lonDelta = radiusKM / (earthRadiusKM * cos) * 180 / math.Pi
	}

	// A hair of slack, because the box is a PRE-FILTER and the exact haversine
	// runs after it. A row on the boundary lost to a floating-point tie is a
	// company missing from the answer; a few extra candidates cost one
	// comparison each.
	const slack = 1e-9
	minLon, maxLon = center.Lon-lonDelta-slack, center.Lon+lonDelta+slack

	// THE BOX MUST NOT WRAP. A circle near the antimeridian spans longitudes on
	// both sides of ±180 — centre 179.9 with a 50km radius reaches -179.9 — and
	// a plain `BETWEEN 179.45 AND 180.35` excludes every one of them. The same
	// happens near a pole, where lonDelta is clamped to 180 and the box runs
	// from -80 to 280: valid rows below -80 fall outside it.
	//
	// Rather than emit a wrapped OR-pair for a case this product may never see,
	// the box widens to the whole globe whenever it would cross the edge. The
	// haversine still decides membership exactly; all that is lost is the
	// index's help, on the few queries that ask about the ends of the earth.
	if minLon < -180 || maxLon > 180 {
		minLon, maxLon = -180, 180
	}
	return minLat - slack, maxLat + slack, minLon, maxLon
}

// distanceSQL renders the great-circle distance in kilometres between a row's
// point and the center, as a SQL expression.
//
// Haversine rather than the law of cosines: the latter loses precision for
// small distances, and small distances — two companies in one city — are
// exactly what this feature is asked about.
func distanceSQL(alias string, columns Columns, centerLat, centerLon string) string {
	return fmt.Sprintf(`(%f * 2 * asin(sqrt(
		power(sin(radians(%s.%s - %s) / 2), 2)
		+ cos(radians(%s)) * cos(radians(%s.%s))
		* power(sin(radians(%s.%s - %s) / 2), 2))))`,
		earthRadiusKM,
		alias, columns.Lat, centerLat,
		centerLat, alias, columns.Lat,
		alias, columns.Lon, centerLon)
}

// LocatedSQL is the term that says a row has a usable point: the coordinates
// match the address the row holds right now, and neither is NULL.
//
// THE STATUS IS NOT OPTIONAL. A row's coordinates belong to whatever address it
// held when the worker last ran, and an address can change at any time. Reading
// lat/lon without the status would answer a distance from where a company USED
// to be, and report success.
func LocatedSQL(alias string, cols Columns, arg func(any) int) []string {
	return []string{
		fmt.Sprintf("%s.%s = $%d", alias, cols.Status, arg(ResolvedStatus)),
		fmt.Sprintf("%s.%s IS NOT NULL AND %s.%s IS NOT NULL", alias, cols.Lat, alias, cols.Lon),
	}
}

// Within renders the distance expression for the projection and the WHERE
// terms that narrow to the radius: LocatedSQL, an indexable bounding box, and
// the haversine that decides membership. Every value binds through arg.
func Within(
	alias string, cols Columns, center Point, radiusKM float64, arg func(any) int,
) (distance string, where []string) {
	minLat, maxLat, minLon, maxLon := boundingBox(center, radiusKM)
	lat := arg(center.Lat)
	lon := arg(center.Lon)

	distance = distanceSQL(alias, cols, fmt.Sprintf("$%d", lat), fmt.Sprintf("$%d", lon))

	where = LocatedSQL(alias, cols, arg)
	return distance, append(where,
		fmt.Sprintf("%s.%s BETWEEN $%d AND $%d", alias, cols.Lat, arg(minLat), arg(maxLat)),
		fmt.Sprintf("%s.%s BETWEEN $%d AND $%d", alias, cols.Lon, arg(minLon), arg(maxLon)),
		fmt.Sprintf("%s <= $%d", distance, arg(radiusKM)),
	)
}

// PlausiblePoint rejects a point that is not on the earth. It is the cheapest
// place to catch a transposed lat/lon pair, which otherwise produces
// confidently wrong distances rather than an error anyone notices.
func PlausiblePoint(p Point) bool {
	return p.Lat >= -90 && p.Lat <= 90 && p.Lon >= -180 && p.Lon <= 180
}
