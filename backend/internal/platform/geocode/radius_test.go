// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package geocode

import (
	"math"
	"strings"
	"testing"
)

// The bounding box must never exclude a row the circle would include.
//
// Too wide costs a few extra comparisons. Too narrow silently drops companies
// within the radius, and the answer looks complete.
func TestTheBoundingBoxNeverCutsInsideTheCircle(t *testing.T) {
	stuttgart := Point{Lat: 48.7758, Lon: 9.1829}
	for _, radiusKM := range []float64{1, 25, 50, 200, 1000} {
		minLat, maxLat, minLon, maxLon := boundingBox(stuttgart, radiusKM)
		// Sample the circle's edge all the way round. Every point at the radius
		// must fall inside the box.
		for degrees := 0; degrees < 360; degrees += 5 {
			edge := destination(stuttgart, radiusKM, float64(degrees))
			if edge.Lat < minLat || edge.Lat > maxLat {
				t.Errorf("radius %vkm bearing %d°: latitude %v outside box [%v, %v]",
					radiusKM, degrees, edge.Lat, minLat, maxLat)
			}
			if edge.Lon < minLon || edge.Lon > maxLon {
				t.Errorf("radius %vkm bearing %d°: longitude %v outside box [%v, %v]",
					radiusKM, degrees, edge.Lon, minLon, maxLon)
			}
		}
	}
}

// Near a pole the longitude box widens without bound, so it is clamped to the
// whole range rather than producing nonsense. Rare, and wrong is worse than
// slow.
func TestTheBoundingBoxSurvivesThePoles(t *testing.T) {
	for _, lat := range []float64{89.9, -89.9, 90, -90} {
		_, _, minLon, maxLon := boundingBox(Point{Lat: lat, Lon: 0}, 100)
		if math.IsNaN(minLon) || math.IsInf(minLon, 0) || math.IsNaN(maxLon) || math.IsInf(maxLon, 0) {
			t.Errorf("latitude %v produced a longitude box [%v, %v]", lat, minLon, maxLon)
		}
		if maxLon-minLon < 359 {
			t.Errorf("latitude %v did not widen to the whole range: [%v, %v]", lat, minLon, maxLon)
		}
	}
}

// destination walks radiusKM from a point along a bearing, so the box test has
// real circle-edge points to check rather than an approximation of them.
func destination(from Point, distanceKM, bearingDegrees float64) Point {
	angular := distanceKM / earthRadiusKM
	bearing := bearingDegrees * math.Pi / 180
	lat1 := from.Lat * math.Pi / 180
	lon1 := from.Lon * math.Pi / 180

	lat2 := math.Asin(math.Sin(lat1)*math.Cos(angular) +
		math.Cos(lat1)*math.Sin(angular)*math.Cos(bearing))
	lon2 := lon1 + math.Atan2(
		math.Sin(bearing)*math.Sin(angular)*math.Cos(lat1),
		math.Cos(angular)-math.Sin(lat1)*math.Sin(lat2))
	// Normalised into [-180, 180], the only form a stored coordinate takes.
	// Without it the test asks the box to hold longitudes no row can have.
	return Point{Lat: lat2 * 180 / math.Pi, Lon: normalizeLon(lon2 * 180 / math.Pi)}
}

// normalizeLon wraps a longitude into [-180, 180].
func normalizeLon(lon float64) float64 {
	for lon > 180 {
		lon -= 360
	}
	for lon < -180 {
		lon += 360
	}
	return lon
}

// A circle spanning the antimeridian, a pole, or the whole earth must not lose
// rows to the bounding box.
//
// The haversine decides membership, but a row the box excludes never reaches
// it. A circle around Stuttgart is far from every edge, so these cases are
// needed.
func TestTheBoundingBoxDoesNotLoseRowsAtTheEdgesOfTheEarth(t *testing.T) {
	for _, c := range []struct {
		name   string
		center Point
		km     float64
	}{
		{"just west of the antimeridian", Point{Lat: 0, Lon: 179.9}, 50},
		{"just east of the antimeridian", Point{Lat: 0, Lon: -179.9}, 50},
		{"near the north pole", Point{Lat: 89.5, Lon: 100}, 200},
		{"near the south pole", Point{Lat: -89.5, Lon: -100}, 200},
		{"larger than the earth", Point{Lat: 48.7758, Lon: 9.1829}, 50000},
	} {
		t.Run(c.name, func(t *testing.T) {
			minLat, maxLat, minLon, maxLon := boundingBox(c.center, c.km)
			for degrees := 0; degrees < 360; degrees += 5 {
				edge := destination(c.center, c.km, float64(degrees))
				if edge.Lat < minLat || edge.Lat > maxLat {
					t.Errorf("bearing %d°: latitude %v outside [%v, %v]",
						degrees, edge.Lat, minLat, maxLat)
				}
				if edge.Lon < minLon || edge.Lon > maxLon {
					t.Errorf("bearing %d°: longitude %v outside [%v, %v] — a company inside the "+
						"circle is excluded before the haversine ever sees it",
						degrees, edge.Lon, minLon, maxLon)
				}
			}
		})
	}
}

func counter() (func(any) int, *[]any) {
	var args []any
	return func(v any) int {
		args = append(args, v)
		return len(args)
	}, &args
}

func TestWithinOnlyReadsLocatedRowsInsideTheBoxAndCircle(t *testing.T) {
	cols, _ := ColumnsFor("company")
	arg, args := counter()
	distance, where := Within("t", cols, Point{Lat: 48.7758, Lon: 9.1829}, 50, arg)
	joined := strings.Join(where, " AND ")
	for _, want := range []string{"t.geocode_status = $", "t.geocode_lat IS NOT NULL", "t.geocode_lon BETWEEN", distance + " <= $"} {
		if !strings.Contains(joined, want) {
			t.Errorf("Within is missing %q:\n%s", want, joined)
		}
	}
	if strings.Contains(joined, "48.7758") || len(*args) == 0 {
		t.Errorf("a value was typed into the SQL instead of bound: %s, args %v", joined, *args)
	}
}

func TestLocatedSQLNamesTheStatusAndBothCoordinates(t *testing.T) {
	cols, _ := ColumnsFor("company")
	arg, args := counter()
	got := strings.Join(LocatedSQL("c", cols, arg), " AND ")
	if got != "c.geocode_status = $1 AND c.geocode_lat IS NOT NULL AND c.geocode_lon IS NOT NULL" {
		t.Errorf("LocatedSQL = %q", got)
	}
	if len(*args) != 1 || (*args)[0] != ResolvedStatus {
		t.Errorf("args = %v, want the resolved status", *args)
	}
}

func TestOnlyCompaniesAreLocatable(t *testing.T) {
	for _, target := range []string{"deal", "contact", "lead"} {
		if _, ok := ColumnsFor(target); ok {
			t.Errorf("%s is locatable", target)
		}
	}
}

func TestPlausiblePointRefusesOffEarthPoints(t *testing.T) {
	for p, want := range map[Point]bool{
		{Lat: 48.7, Lon: 9.1}: true, {Lat: 181, Lon: 9}: false, {Lat: 0, Lon: -181}: false, {Lat: -90, Lon: 180}: true,
	} {
		if PlausiblePoint(p) != want {
			t.Errorf("PlausiblePoint(%v) = %v", p, !want)
		}
	}
}
