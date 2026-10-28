// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package search

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/geocode"
)

// A record type that is not somewhere cannot be asked about by distance, and
// the answer says so rather than pretending.
func TestARecordTypeWithNoPlaceAnswersUnavailable(t *testing.T) {
	km := 50.0
	for _, target := range []string{"deal", "contact", "lead", "project"} {
		bound, note, err := bindGeo(context.Background(), stubPlaces{},
			target, "address", radiusOperand{Center: "Stuttgart", RadiusKM: &km})
		if err != nil {
			t.Fatalf("%s: %v", target, err)
		}
		if bound != nil {
			t.Errorf("%s bound a radius predicate; only a company is somewhere", target)
		}
		if note == nil || note.Code != CodeDistanceRankingUnavailable {
			t.Errorf("%s did not answer the honest unavailable note: %+v", target, note)
		}
	}
}

// A place nothing has looked up answers a note, NOT an outbound request.
// query_workspace is declared workspace-local and the cap is derived from that
// declaration; reaching a geocoder here would make the declaration a lie.
func TestAnUnknownPlaceAnswersANoteRatherThanLeavingTheWorkspace(t *testing.T) {
	km := 50.0
	bound, note, err := bindGeo(context.Background(), stubPlaces{},
		"company", "address", radiusOperand{Center: "Atlantis", RadiusKM: &km})
	if err != nil {
		t.Fatalf("binding: %v", err)
	}
	if bound != nil {
		t.Error("a place the cache does not hold was bound anyway")
	}
	if note == nil {
		t.Error("an unresolvable place produced no note, so the caller is told nothing")
	}
}

// Coordinates given directly need no cache at all: a caller holding a point has
// already done the resolving.
func TestAPointGivenDirectlyNeedsNoCache(t *testing.T) {
	km, lat, lon := 50.0, 48.7758, 9.1829
	bound, note, err := bindGeo(context.Background(), nil,
		"company", "address", radiusOperand{Lat: &lat, Lon: &lon, RadiusKM: &km})
	if err != nil {
		t.Fatalf("binding: %v", err)
	}
	if note != nil {
		t.Errorf("an explicit point produced a note: %+v", note)
	}
	if bound == nil {
		t.Fatal("an explicit point did not bind")
	}
	if bound.Center.Lat != lat || bound.Center.Lon != lon {
		t.Errorf("center = %+v, want the point the caller gave", bound.Center)
	}
}

// The operand says WHERE exactly once. A name and a point together have two
// answers and no way to say which was meant; half a point is not a point.
func TestTheOperandNamesACenterExactlyOnce(t *testing.T) {
	lat, lon := 48.7758, 9.1829
	for name, operand := range map[string]radiusOperand{
		"neither":             {},
		"both name and point": {Center: "Stuttgart", Lat: &lat, Lon: &lon},
		"latitude alone":      {Lat: &lat},
		"longitude alone":     {Lon: &lon},
	} {
		if operand.namesACenter() {
			t.Errorf("%s was accepted as a center", name)
		}
	}
	if !(radiusOperand{Center: "Stuttgart"}).namesACenter() {
		t.Error("a place name alone was not accepted as a center")
	}
	if !(radiusOperand{Lat: &lat, Lon: &lon}).namesACenter() {
		t.Error("a point alone was not accepted as a center")
	}
}

// A center off the earth is refused: it is where a transposed lat/lon pair gets
// caught, and a transposition produces confidently wrong distances rather than
// an error anyone notices.
func TestACenterOffTheEarthIsRefused(t *testing.T) {
	bad, fine := 181.0, 9.1829
	if (radiusOperand{Lat: &bad, Lon: &fine}).plausibleCenter() {
		t.Error("a latitude of 181 was accepted")
	}
	lat := 48.7758
	if !(radiusOperand{Lat: &lat, Lon: &fine}).plausibleCenter() {
		t.Error("Stuttgart was rejected as implausible")
	}
}

// stubPlaces is a cache holding one city, so an unknown name is genuinely
// unknown rather than merely unwired.
type stubPlaces struct{}

func (stubPlaces) LookupPlace(_ context.Context, query string) (Point, bool, error) {
	if query == "Stuttgart" {
		return Point{Lat: 48.7758, Lon: 9.1829}, true, nil
	}
	return Point{}, false, nil
}

// The SQL a radius compiles to must read the point's status, not the
// coordinates alone.
//
// This is the whole staleness design showing up in one predicate. A company's
// coordinates belong to whatever address it held when the worker last ran, and
// an address can change at any time — so a query reading lat/lon without the
// status would answer a distance from where the company USED to be, and report
// success while doing it.
func TestARadiusOnlyReadsCoordinatesThatMatchTheAddress(t *testing.T) {
	c := &planCompiler{}
	distance, where := c.radius("t", geoBinding{
		Center:   Point{Lat: 48.7758, Lon: 9.1829},
		RadiusKM: 50,
		Columns:  companyColumns(t),
		Field:    "address",
	})
	joined := strings.Join(where, " AND ")

	if !strings.Contains(joined, "located_point.status") {
		t.Errorf("the radius does not check the point's status, so a moved company answers from its old "+
			"address:\n%s", joined)
	}
	if !strings.Contains(joined, "IS NOT NULL") {
		t.Errorf("the radius does not exclude null coordinates; a NULL in the haversine is silently "+
			"false rather than an error:\n%s", joined)
	}
	if !strings.Contains(joined, "BETWEEN") {
		t.Errorf("the radius has no bounding box, so every query is a full scan — the haversine "+
			"cannot use an index:\n%s", joined)
	}
	if distance == "" {
		t.Error("the radius produced no distance expression, so the answer cannot say how far")
	}
	if !strings.Contains(joined, "asin(sqrt(") || !strings.Contains(distance, "asin(sqrt(") {
		t.Error("the haversine does not decide membership; the box alone is wider " +
			"than the circle and would admit companies outside the radius")
	}
}

// Every value reaches SQL as a bound parameter. A centre is caller-supplied
// text, and a radius that interpolated it would be an injection door on a
// read tool.
func TestARadiusBindsEveryValueRatherThanInterpolatingIt(t *testing.T) {
	c := &planCompiler{}
	_, where := c.radius("t", geoBinding{
		Center:   Point{Lat: 48.7758, Lon: 9.1829},
		RadiusKM: 50,
		Columns:  companyColumns(t),
	})
	joined := strings.Join(where, " AND ")
	if strings.Contains(joined, "48.7758") || strings.Contains(joined, "9.1829") {
		t.Errorf("a coordinate was interpolated into the SQL rather than bound:\n%s", joined)
	}
	if len(c.args) == 0 {
		t.Error("the radius bound no parameters at all")
	}
}

// A radius inside a TRAVERSAL refuses rather than being dropped.
//
// The root radius is rendered by planCompiler.radius and skipped by the
// ordinary clause loop. That same loop compiles a traversal's predicates, and
// there nothing binds a radius — so skipping it there would drop the predicate
// silently and return EVERY related record. A wider answer in the shape of the
// right one is the worst outcome available, and it is what this refusal
// prevents until hop binding exists.
func TestARadiusInsideATraversalRefusesRatherThanBeingDropped(t *testing.T) {
	c := &planCompiler{}
	km := 50.0
	operand, err := json.Marshal(radiusOperand{Center: "Stuttgart", RadiusKM: &km})
	if err != nil {
		t.Fatalf("building the operand: %v", err)
	}
	clauses := []Predicate{{Field: "address", Op: OpWithinRadius, Value: operand}}

	fragments, refusals, err := c.predicates(t.Context(), "h", unfilteredStorage(),
		TargetVocabulary{Target: "company"}, "traverse.where", clauses, false)
	if err != nil {
		t.Fatalf("compiling the hop predicates: %v", err)
	}
	if len(fragments) != 0 {
		t.Errorf("a hop radius compiled to %v; nothing binds it there", fragments)
	}
	if len(refusals) != 1 {
		t.Fatalf("a hop radius produced %d refusals; it must refuse exactly once rather than "+
			"pass silently and widen the answer", len(refusals))
	}
	if refusals[0].Code != CodeDistanceRankingUnavailable {
		t.Errorf("the refusal answers %q, want %q", refusals[0].Code, CodeDistanceRankingUnavailable)
	}

	// The ROOT is the opposite: skipped here on purpose, because radius()
	// renders it. Neither a fragment nor a refusal.
	fragments, refusals, err = c.predicates(t.Context(), "t", unfilteredStorage(),
		TargetVocabulary{Target: "company"}, "where", clauses, true)
	if err != nil {
		t.Fatalf("compiling the root predicates: %v", err)
	}
	if len(fragments) != 0 || len(refusals) != 0 {
		t.Errorf("the root radius produced fragments %v and refusals %v; radius() renders it",
			fragments, refusals)
	}
}

// A second radius refuses rather than being ignored.
//
// The clauses are ANDed, so ignoring one widens the answer without changing its
// shape: "near Stuttgart and near Munich" would return everything near
// Stuttgart, reported as a complete exact answer.
func TestASecondRadiusRefusesRatherThanWideningTheAnswer(t *testing.T) {
	km := 50.0
	operand, err := json.Marshal(radiusOperand{Center: "Stuttgart", RadiusKM: &km})
	if err != nil {
		t.Fatalf("building the operand: %v", err)
	}
	one := Predicate{Field: "address", Op: OpWithinRadius, Value: operand}

	if at, found := secondRadius([]Predicate{one}); found {
		t.Errorf("a single radius was reported as a second one, at %q", at)
	}
	at, found := secondRadius([]Predicate{one, one})
	if !found {
		t.Fatal("two radius clauses were accepted; the answer would be every company inside the FIRST circle")
	}
	if at != "where[1]" {
		t.Errorf("the refusal points at %q rather than the second clause", at)
	}
}

func companyColumns(t *testing.T) geocode.Columns {
	t.Helper()
	cols, ok := geocode.ColumnsFor("company")
	if !ok {
		t.Fatal("company is not a locatable record type")
	}
	return cols
}
