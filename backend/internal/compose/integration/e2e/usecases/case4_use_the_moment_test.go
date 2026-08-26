// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package usecases

// CASE 4 — Use the moment.
//
// The prompt, from the run this was written against:
//
//	hey I am in cologne right now and wanted to visit a client but my
//	appointment got canceled. Anyone nearby in our crm I should visit?
//
// It never says "companies", never says "address", never says "distance", and
// names no tool. What the assistant has to work out for itself is the whole
// case: that "nearby" means a distance search, that a distance search needs a
// geo field, and that it can ASK which fields are geo. This suite pins the
// three answers that chain depends on, in the order the assistant meets them.
//
// No geocoder runs here, and none ever will. The city centre resolves from the
// caller's own located companies, so seeding companies with coordinates is what
// makes "Cologne" a point. If a real geocoding provider ever becomes the
// PRIMARY lookup, this suite starts reaching the network — which is a reason to
// fail loudly, not a reason to soften the fixture.

import (
	"encoding/json"
	"testing"

	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/modules/search"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Cologne cathedral, and Munich as the company no plausible radius admits.
//
// The centre a radius measures from is NOT one of these points. `LookupCity`
// averages the caller's own located companies filed under that city name, so
// "Köln" resolves to the midpoint of whatever this fixture seeded there. That
// is why the assertions below are about ORDER and MEMBERSHIP rather than about
// a company being exactly N km out — a distance that depends on the fixture's
// own average is a number the test would be checking against itself.
//
// The city name on each row is load-bearing for the same reason. Filing Munich
// under 'Köln' would stretch the average across ~2.8 degrees, over the one-degree
// spread cap that stops "Frankfurt" resolving to a point between two of them —
// and the resolver would correctly refuse the whole query.
const (
	cologneLat = 50.9375
	cologneLon = 6.9603
	munichLat  = 48.1351
	munichLon  = 11.5820
)

// seedLocatedOrg inserts a company that has already been geocoded: an address,
// a point, and the 'ok' status saying the two agree.
//
// The owner is a parameter and not a default, because criteria 4 and 5 are
// about the DIFFERENCE between an account the caller owns and a colleague's.
// A fixture where everything belongs to one seat cannot fail those criteria and
// therefore cannot pass them.
func (s *scenario) seedLocatedOrg(t *testing.T, name, city string, lat, lon float64, owner ids.UUID) ids.UUID {
	t.Helper()
	return s.seedID(t, `INSERT INTO organization
		(id, owner_id, display_name, address_line1, address_city,
		 geocode_lat, geocode_lon, geocode_status, geocode_provider, geocode_input_hash,
		 source, captured_by)
		VALUES ($1, $2, $3, 'Hauptstrasse 1', $4, $5, $6, 'ok', 'test', 'seeded', 'manual', 'human:x')`,
		owner, name, city, lat, lon)
}

// queryPlan runs one plan and decodes the answer.
func (s *scenario) queryPlan(t *testing.T, plan string) agents.QueryWorkspaceResult {
	t.Helper()
	var raw json.RawMessage = []byte(plan)
	got := s.MCP.CallOK(t, "query_workspace", map[string]any{"plan": raw})
	var result agents.QueryWorkspaceResult
	got.JSON(t, &result)
	return result
}

// TestCase4TheAssistantCanDiscoverThatAddressesAreSearchableByDistance pins
// criterion 1: the assistant finds out the CRM knows about locations without
// being told.
//
// This is the first link in the chain and the one that was missing until
// #2728: the grammar was served and unreachable to a tools-only client, so an
// assistant had no way to learn that a radius search existed at all.
func TestCase4TheAssistantCanDiscoverThatAddressesAreSearchableByDistance(t *testing.T) {
	s := boot(t, scopesRead)

	got := s.MCP.CallOK(t, "describe_query_vocabulary", nil)
	if !containsAll(got.Text, "address", "within_radius") {
		t.Fatalf("case 4 criterion 1: the vocabulary does not tell a caller that `address` "+
			"supports `within_radius`, so an assistant asked for who is NEARBY has no way to "+
			"learn a distance search exists:\n%s", got.Text)
	}
}

// TestCase4NearbyCompaniesComeBackNearestFirstAndSayHowComplete pins criteria
// 2, 3 and 7: the radius answers, it is ordered, it says whether the list is
// everything, and an unanswerable radius says so rather than returning an
// unordered list that looks ordered.
func TestCase4NearbyCompaniesComeBackNearestFirstAndSayHowComplete(t *testing.T) {
	s := boot(t, scopesRead)

	// Three inside a 50km radius at roughly 0km, 12km and 27km, plus Munich at
	// ~460km which must be absent.
	s.seedLocatedOrg(t, "Dom Digital GmbH", "Köln", cologneLat, cologneLon, s.Rep)
	s.seedLocatedOrg(t, "Rheinufer AG", "Köln", cologneLat+0.11, cologneLon, s.Colleague)
	s.seedLocatedOrg(t, "Vorort Systeme KG", "Köln", cologneLat+0.25, cologneLon, s.Colleague)
	s.seedLocatedOrg(t, "München Ferne GmbH", "München", munichLat, munichLon, s.Rep)

	result := s.queryPlan(t, `{
		"version": "v1", "target": "organization",
		"where": [{"field": "address", "op": "within_radius",
		           "value": {"center": "Köln", "radius_km": 50}}]}`)

	if len(result.Rows) != 3 {
		t.Fatalf("case 4 criterion 2: a 50km radius around Köln admitted %d companies, want the 3 "+
			"inside it (Munich is ~460km away and must not appear):\n%s", len(result.Rows), result.ExecutedPlan)
	}

	// Criterion 2's second half: nearest first. Reading the distances in order
	// is the assertion — a list that is right but unordered sends a rep to the
	// far one first.
	var previous float64
	for i, row := range result.Rows {
		if row.DistanceKM == nil {
			t.Fatalf("case 4 criterion 2: row %d (%s) carries no distance, so a caller cannot tell "+
				"how far it is or trust the order", i, recordName(t, row.Record.Fields))
		}
		if *row.DistanceKM < previous {
			t.Fatalf("case 4 criterion 2: row %d (%s) is %.1fkm out, closer than the row before it "+
				"at %.1fkm — the answer is not nearest-first", i, recordName(t, row.Record.Fields), *row.DistanceKM, previous)
		}
		previous = *row.DistanceKM
	}

	// Criterion 3: "these are all of them" and "these are some of them" are
	// different answers, and a rep deciding whether to look further needs to
	// know which one they were given.
	if result.Coverage != agents.CoverageCompleteExact {
		t.Fatalf("case 4 criterion 3: coverage is %q, want %q — an exact radius over a small corpus "+
			"is a complete answer, and anything else tells the rep to keep looking",
			result.Coverage, agents.CoverageCompleteExact)
	}
}

// TestCase4ARadiusItCannotMeasureSaysSoInsteadOfAnsweringAnyway pins criterion
// 7. A person HAS an address, so both the field and the operator exist and the
// plan looks answerable — but this product does not geocode where people live,
// so there is nothing to measure from.
func TestCase4ARadiusItCannotMeasureSaysSoInsteadOfAnsweringAnyway(t *testing.T) {
	s := boot(t, scopesRead)
	s.seedLocatedOrg(t, "Dom Digital GmbH", "Köln", cologneLat, cologneLon, s.Rep)

	result := s.queryPlan(t, `{
		"version": "v1", "target": "person",
		"where": [{"field": "address", "op": "within_radius",
		           "value": {"center": "Köln", "radius_km": 50}}]}`)

	if len(result.Rows) != 0 {
		t.Fatalf("case 4 criterion 7: a radius the product cannot measure returned %d rows, which "+
			"a caller would read as an ordered nearby list", len(result.Rows))
	}
	if !hasNote(result, search.CodeDistanceRankingUnavailable) {
		t.Fatalf("case 4 criterion 7: an unmeasurable radius returned no note saying so; notes are %v",
			result.Notes)
	}
}

// TestCase4EveryCompanySaysWhoOwnsItByName pins criteria 4 and 5, and it is the
// criterion this case exists for.
//
// Run 1 of the real scenario returned seven correct companies and never said
// Sofia Meier owned one of them, because the row carried owner_id as a bare
// uuid — data present, correct, and useless to the person reading it. Two
// thirds of the accounts in the demo CRM belong to somebody other than the
// caller, so this is the common case rather than an edge.
func TestCase4EveryCompanySaysWhoOwnsItByName(t *testing.T) {
	s := boot(t, scopesRead)

	mine := s.seedLocatedOrg(t, "Dom Digital GmbH", "Köln", cologneLat, cologneLon, s.Rep)
	theirs := s.seedLocatedOrg(t, "Rheinufer AG", "Köln", cologneLat+0.11, cologneLon, s.Colleague)

	result := s.queryPlan(t, `{
		"version": "v1", "target": "organization",
		"where": [{"field": "address", "op": "within_radius",
		           "value": {"center": "Köln", "radius_km": 50}}]}`)

	seen := map[ids.UUID]agents.RecordOwner{}
	for _, row := range result.Rows {
		if row.Owner == nil {
			t.Fatalf("case 4 criterion 4: %s came back with no owner block at all, so a rep cannot "+
				"tell whose account it is", recordName(t, row.Record.Fields))
		}
		if row.Owner.Name == "" {
			t.Fatalf("case 4 criterion 4: %s names its owner only as %s — an id is not something a "+
				"rep can act on", recordName(t, row.Record.Fields), row.Owner.ID)
		}
		seen[row.Record.ID] = *row.Owner
	}

	own, ok := seen[mine]
	if !ok {
		t.Fatalf("case 4: the company the caller owns is missing from the answer")
	}
	if !own.IsYou {
		t.Fatalf("case 4 criterion 5: the caller's OWN account came back marked as somebody else's "+
			"(owner %s, is_you=false)", own.Name)
	}
	if own.Name != repName {
		t.Fatalf("case 4 criterion 4: the caller's own account names its owner %q, want %q", own.Name, repName)
	}

	colleague, ok := seen[theirs]
	if !ok {
		t.Fatalf("case 4: the colleague's company is missing from the answer")
	}
	if colleague.IsYou {
		t.Fatalf("case 4 criterion 5: a colleague's account came back marked as the caller's own, " +
			"which is how a rep walks into somebody else's meeting")
	}
	if colleague.Name != colleagueName {
		t.Fatalf("case 4 criterion 5: the colleague's account names its owner %q, want %q — this is "+
			"the name the assistant has to say out loud before the rep knocks",
			colleague.Name, colleagueName)
	}
}

// hasNote says whether the answer carries a note with this code.
func hasNote(result agents.QueryWorkspaceResult, code string) bool {
	for _, note := range result.Notes {
		if note.Code == code {
			return true
		}
	}
	return false
}
