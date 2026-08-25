// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package migration

// What a delimited file offers as edges, and — more importantly — what it does
// not.

import (
	"context"
	"testing"
)

func personEmployerSource(t *testing.T, body string) *CSVSource {
	t.Helper()
	mapping := map[string]string{
		"Email":   "email",
		"Name":    "full_name",
		"Company": AssocTargetOrganizationName,
	}
	return NewCSVSource(seedCSV(t, body), testCSVKey, ObjectPerson, mapping, "Email")
}

// One edge per row naming a company, carrying the row's own source key so the
// writer can find the person it landed.
func TestAPersonFileOffersOneEmployerEdgePerNamedCompany(t *testing.T) {
	src := personEmployerSource(t, "Email,Name,Company\n"+
		"ada@x.test,Ada,Analytical Engines\n"+
		"grace@x.test,Grace,\n"+
		"joan@x.test,Joan,Bletchley Ltd\n")

	edges, err := src.Associations(context.Background())
	if err != nil {
		t.Fatalf("Associations: %v", err)
	}
	if len(edges) != 2 {
		t.Fatalf("edges = %d, want one per row naming a company — the empty cell offers none", len(edges))
	}
	if edges[0].FromID != "ada@x.test" || edges[0].ToID != "Analytical Engines" {
		t.Fatalf("edge = %+v, want the row's source key and the company as written", edges[0])
	}
	if edges[0].FromType != ObjectPerson || edges[0].ToType != AssocTargetOrganizationName {
		t.Fatalf("edge endpoints = %s→%s, want a person and a company NAME", edges[0].FromType, edges[0].ToType)
	}
	if edges[0].Category != assocCategoryEmployment {
		t.Fatalf("category = %q, want employment", edges[0].Category)
	}
}

// The rule that stops a wrong human being attached to a company.
//
// Two rows claiming one source key: Rows delivers only the FIRST, so only one
// person is landed and the identity map holds it under that key. An edge emitted
// for the second row would resolve its person endpoint to the first row's person
// — attaching Ada to Grace's employer, silently, with every count still adding
// up. It has no natural symptom, which is why it has a test.
func TestARowWhoseKeyWasAlreadyClaimedOffersNoEdge(t *testing.T) {
	src := personEmployerSource(t, "Email,Name,Company\n"+
		"ada@x.test,Ada,Analytical Engines\n"+
		"ada@x.test,Grace,Bletchley Ltd\n")

	edges, err := src.Associations(context.Background())
	if err != nil {
		t.Fatalf("Associations: %v", err)
	}
	if len(edges) != 1 {
		t.Fatalf("edges = %d, want only the delivered row's — the second row lands no person to link", len(edges))
	}
	if edges[0].ToID != "Analytical Engines" {
		t.Fatalf("edge = %+v, want the FIRST row's company; the second row's would attach the wrong employer", edges[0])
	}
}

// A row with no value in the source-key column is never delivered, so it can
// have no edge either.
func TestARowWithNoSourceKeyOffersNoEdge(t *testing.T) {
	src := personEmployerSource(t, "Email,Name,Company\n"+
		",Nobody,Analytical Engines\n"+
		"ada@x.test,Ada,Bletchley Ltd\n")

	edges, err := src.Associations(context.Background())
	if err != nil {
		t.Fatalf("Associations: %v", err)
	}
	if len(edges) != 1 || edges[0].FromID != "ada@x.test" {
		t.Fatalf("edges = %+v, want only the row that can be identified", edges)
	}
}

// A file that maps no company column asks for no links, and neither does a run
// importing anything but people.
func TestNoEmployerColumnAndNoPersonRunOfferNoEdges(t *testing.T) {
	unmapped := NewCSVSource(seedCSV(t, "Email,Name\nada@x.test,Ada\n"), testCSVKey,
		ObjectPerson, map[string]string{"Email": "email", "Name": "full_name"}, "Email")
	edges, err := unmapped.Associations(context.Background())
	if err != nil {
		t.Fatalf("Associations: %v", err)
	}
	if len(edges) != 0 {
		t.Fatalf("edges = %d, want none — the file mapped no company column", len(edges))
	}

	orgs := NewCSVSource(seedCSV(t, "Company\nAcme\n"), testCSVKey,
		ObjectOrganization, map[string]string{"Company": "display_name"}, "Company")
	edges, err = orgs.Associations(context.Background())
	if err != nil {
		t.Fatalf("Associations: %v", err)
	}
	if len(edges) != 0 {
		t.Fatalf("edges = %d, want none — a company file has no employer to name", len(edges))
	}
}
