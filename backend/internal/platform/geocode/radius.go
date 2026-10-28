// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package geocode

import (
	"fmt"
	"math"
	"slices"
	"strings"
)

// Columns is where a record type's point lives. It names a side table keyed by
// the record's id, the coordinate pair, and the status saying whether the pair
// means anything. The point is kept off the record so a lookup never moves the
// record's version.
type Columns struct {
	Table, Key, Lat, Lon, Status string
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
	"company": {Table: "company_geocode_state", Key: "company_id", Lat: "lat", Lon: "lon", Status: "status"},
}

// ColumnsFor returns the coordinate columns of a record type, and false for a
// type that is not somewhere.
func ColumnsFor(target string) (Columns, bool) {
	cols, ok := locatable[target]
	return cols, ok
}

// earthRadiusKM is the mean radius a haversine over a sphere takes. Over the
// distances a sales team asks about, the error is well under the error in
// "which office is this company at".
const earthRadiusKM = 6371.0

// boundingBox is the cheap pre-filter a radius query runs before any
// trigonometry.
//
// The haversine is exact but Postgres cannot index it, so the box narrows the
// candidates for the index to find. The box is wider than the circle. It must
// never exclude a row the circle includes; extra rows cost one comparison each
// before the haversine rejects them.
func boundingBox(center Point, radiusKM float64) (minLat, maxLat, minLon, maxLon float64) {
	latDelta := radiusKM / earthRadiusKM * 180 / math.Pi
	minLat, maxLat = center.Lat-latDelta, center.Lat+latDelta

	// The cosine is taken at the widest latitude the circle reaches, not at the
	// center. Longitude degrees narrow toward the poles, so a circle reaching
	// further from the equator bulges further east and west than the center's
	// cosine predicts. A box from the center's cosine would cut inside the
	// circle and drop companies within the radius.
	//
	// The widest latitude is whichever box edge sits further from the equator,
	// since |cos| shrinks as |latitude| grows.
	widest := math.Max(math.Abs(minLat), math.Abs(maxLat))
	// Near a pole cos() approaches zero and the box would widen without bound,
	// so it is clamped to the whole range. A radius query near a pole falls
	// back to the haversine alone, which is correct and rare.
	cos := math.Cos(math.Min(widest, 90) * math.Pi / 180)
	lonDelta := 180.0
	if cos > 0.01 {
		lonDelta = radiusKM / (earthRadiusKM * cos) * 180 / math.Pi
	}

	// Slack for the pre-filter: a boundary row lost to a floating-point tie
	// would be a company missing from the answer.
	const slack = 1e-9
	minLon, maxLon = center.Lon-lonDelta-slack, center.Lon+lonDelta+slack

	// The box must not wrap. A circle near the antimeridian spans longitudes on
	// both sides of ±180, and `BETWEEN 179.45 AND 180.35` excludes every one on
	// the far side. Near a pole lonDelta is clamped to 180, so the box runs from
	// -80 to 280 and valid rows below -80 fall outside it.
	//
	// The box widens to the whole globe whenever it would cross the edge. The
	// haversine still decides membership; only the index's help is lost, on the
	// few queries that ask about the ends of the earth.
	if minLon < -180 || maxLon > 180 {
		minLon, maxLon = -180, 180
	}
	return minLat - slack, maxLat + slack, minLon, maxLon
}

// distanceSQL renders the great-circle distance in kilometres between a row's
// point and the center, as a SQL expression.
//
// Haversine rather than the law of cosines: the latter loses precision for
// small distances, such as two companies in one city.
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

// ResolvedPointSQL is the terms that say the point row aliased point is usable.
// Its coordinates match the address the record holds now, and neither is NULL.
//
// The status is required. A point belongs to the address the record held when
// the worker last ran, and an address can change at any time. Reading lat/lon
// without the status answers a distance from the previous address and reports
// success.
func ResolvedPointSQL(point string, cols Columns, arg func(any) int) []string {
	return []string{
		fmt.Sprintf("%s.%s = $%d", point, cols.Status, arg(ResolvedStatus)),
		fmt.Sprintf("%s.%s IS NOT NULL AND %s.%s IS NOT NULL", point, cols.Lat, point, cols.Lon),
	}
}

// pointAlias names the point row inside the subqueries below.
const pointAlias = "located_point"

// pointOf renders a subquery over the record's point row with these terms.
func pointOf(alias string, cols Columns, selected string, terms []string) string {
	return fmt.Sprintf("(SELECT %s FROM %s %s WHERE %s.%s = %s.id AND %s)",
		selected, cols.Table, pointAlias, pointAlias, cols.Key, alias, strings.Join(terms, " AND "))
}

// LocatedSQL is the term that says the record aliased alias has a usable point.
func LocatedSQL(alias string, cols Columns, arg func(any) int) []string {
	return []string{"EXISTS " + pointOf(alias, cols, "1", ResolvedPointSQL(pointAlias, cols, arg))}
}

// Within renders the distance expression for the projection and the `WHERE`
// terms that narrow to the radius. Those are LocatedSQL, an indexable bounding
// box, and the haversine that decides membership. Every value binds through arg.
func Within(
	alias string, cols Columns, center Point, radiusKM float64, arg func(any) int,
) (distance string, where []string) {
	minLat, maxLat, minLon, maxLon := boundingBox(center, radiusKM)
	lat := arg(center.Lat)
	lon := arg(center.Lon)

	between := distanceSQL(pointAlias, cols, fmt.Sprintf("$%d", lat), fmt.Sprintf("$%d", lon))
	// One set of point terms serves both subqueries, so every bound value is
	// referenced whether or not the caller projects the distance.
	located := ResolvedPointSQL(pointAlias, cols, arg)
	distance = pointOf(alias, cols, between, located)

	terms := append(slices.Clone(located),
		fmt.Sprintf("%s.%s BETWEEN $%d AND $%d", pointAlias, cols.Lat, arg(minLat), arg(maxLat)),
		fmt.Sprintf("%s.%s BETWEEN $%d AND $%d", pointAlias, cols.Lon, arg(minLon), arg(maxLon)),
		fmt.Sprintf("%s <= $%d", between, arg(radiusKM)),
	)
	return distance, []string{"EXISTS " + pointOf(alias, cols, "1", terms)}
}

// PlausiblePoint rejects a point that is not on the earth. It catches a
// transposed lat/lon pair, which otherwise yields wrong distances and no error.
func PlausiblePoint(p Point) bool {
	return p.Lat >= -90 && p.Lat <= 90 && p.Lon >= -180 && p.Lon <= 180
}
