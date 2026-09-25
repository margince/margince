// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// Naming a record in words, and what happens when the words are ambiguous.
//
// The ambiguity case is the one worth the test. Before this, a host resolved the
// name itself and passed an id, so a name matching two companies produced a
// briefing that was confidently correct about the wrong one — a wrong answer
// nothing in the response marked as a guess. A refusal naming the candidates is
// strictly better, and it is what these assert.

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// searchingProvider answers a fixed set of hits and records what it was asked.
type searchingProvider struct {
	datasource.SystemOfRecordProvider
	hits  []datasource.Record
	asked datasource.SearchQuery
	err   error
}

func (p *searchingProvider) Search(
	_ context.Context, q datasource.SearchQuery,
) (datasource.SearchResult, error) {
	p.asked = q
	if p.err != nil {
		return datasource.SearchResult{}, p.err
	}
	return datasource.SearchResult{Records: p.hits}, nil
}

// aCompany is a hit of the type every case here anchors on. Typed rather than
// parameterised: a parameter only ever given one value reads as a choice the
// tests make and do not.
func aCompany() datasource.Record {
	return datasource.Record{Ref: datasource.EntityRef{Type: "company", ID: ids.NewV7()}}
}

// An id given outright is used as given: naming the record precisely must not
// cost a search.
func TestAnAnchorGivenAnIDDoesNotSearch(t *testing.T) {
	p := &searchingProvider{}
	want := ids.NewV7()

	got, err := resolveAnchor(context.Background(), p,
		anchorArgs{RecordType: "company", RecordID: want})
	if err != nil {
		t.Fatalf("resolveAnchor: %v", err)
	}
	if got != want {
		t.Errorf("resolved to %s, want the id the caller gave (%s)", got, want)
	}
	if p.asked.Text != "" {
		t.Errorf("searched for %q when the caller had already named the record", p.asked.Text)
	}
}

// One match is the record. The search is scoped to the type the caller named,
// so a company called Contoso does not resolve a contact of the same name.
func TestAnAnchorNamedInWordsResolvesToItsOneMatch(t *testing.T) {
	hit := aCompany()
	p := &searchingProvider{hits: []datasource.Record{hit}}

	got, err := resolveAnchor(context.Background(), p,
		anchorArgs{RecordType: "company", RecordName: "Contoso"})
	if err != nil {
		t.Fatalf("resolveAnchor: %v", err)
	}
	if got != hit.Ref.ID {
		t.Errorf("resolved to %s, want the single match %s", got, hit.Ref.ID)
	}
	if p.asked.Text != "Contoso" {
		t.Errorf("searched for %q, want the caller's own words", p.asked.Text)
	}
	if len(p.asked.EntityTypes) != 1 || p.asked.EntityTypes[0] != "company" {
		t.Errorf("searched types %v, want only the type the caller named", p.asked.EntityTypes)
	}
}

// Several matches are REFUSED with the candidates, never guessed. This is the
// whole point: a guess here produces a briefing about the wrong record that
// reads exactly like a briefing about the right one.
func TestAnAmbiguousNameIsRefusedWithItsCandidates(t *testing.T) {
	first, second := aCompany(), aCompany()
	p := &searchingProvider{hits: []datasource.Record{first, second}}

	_, err := resolveAnchor(context.Background(), p,
		anchorArgs{RecordType: "company", RecordName: "Contoso"})

	var bad *BadArgsError
	if !errors.As(err, &bad) {
		t.Fatalf("err = %v, want a BadArgsError a host can act on", err)
	}
	for _, want := range []ids.UUID{first.Ref.ID, second.Ref.ID} {
		if !strings.Contains(err.Error(), want.String()) {
			t.Errorf("the refusal does not name candidate %s: %v", want, err)
		}
	}
	// It says which argument to use, because a refusal that does not is one an
	// agent answers by retrying the same call.
	if !strings.Contains(err.Error(), "record_id") {
		t.Errorf("the refusal does not say how to disambiguate: %v", err)
	}
}

// No match is refused as no match, rather than resolving to the zero uuid — an
// id that names nothing reaches a store, matches no row, and tells the caller a
// record they never mentioned does not exist.
func TestANameMatchingNothingIsRefused(t *testing.T) {
	p := &searchingProvider{}

	got, err := resolveAnchor(context.Background(), p,
		anchorArgs{RecordType: "company", RecordName: "Nobody"})

	var bad *BadArgsError
	if !errors.As(err, &bad) {
		t.Fatalf("err = %v, want a BadArgsError", err)
	}
	if got != (ids.UUID{}) {
		t.Errorf("returned id %s beside an error", got)
	}
}

// The candidate list carries ids and nothing else. The refusal travels to an
// agent that may hold no grant on any of them, and a name or a field would
// publish what the record says to somebody who asked only whether a word was
// ambiguous.
func TestTheCandidateListPublishesNoRecordContent(t *testing.T) {
	first, second := aCompany(), aCompany()
	first.Fields = []byte(`{"name":"Contoso Pharmaceuticals","owner":"a colleague"}`)
	p := &searchingProvider{hits: []datasource.Record{first, second}}

	_, err := resolveAnchor(context.Background(), p,
		anchorArgs{RecordType: "company", RecordName: "Contoso"})

	if err == nil {
		t.Fatal("an ambiguous name must be refused")
	}
	for _, leaked := range []string{"Pharmaceuticals", "a colleague"} {
		if strings.Contains(err.Error(), leaked) {
			t.Errorf("the refusal published record content (%q): %v", leaked, err)
		}
	}
}

// Naming a record both ways is refused rather than resolved from the id: a
// request carrying both is one whose author believed they agreed, and answering
// from the id hides the disagreement on the call where it could still be seen.
func TestNamingARecordBothWaysIsRefused(t *testing.T) {
	err := anchorArgs{RecordType: "company", RecordID: ids.NewV7(), RecordName: "Contoso"}.validate()

	var bad *BadArgsError
	if !errors.As(err, &bad) {
		t.Fatalf("err = %v, want a BadArgsError", err)
	}
}

// Naming it neither way is refused too. `record_id` left the schema's required
// list when `record_name` arrived, so the surface-wide id check no longer makes
// this claim and this is where it is made.
func TestNamingARecordNoWayIsRefused(t *testing.T) {
	for _, args := range []anchorArgs{
		{RecordType: "company"},
		{RecordType: "company", RecordName: "   "},
	} {
		var bad *BadArgsError
		if err := args.validate(); !errors.As(err, &bad) {
			t.Errorf("%+v: err = %v, want a BadArgsError", args, err)
		}
	}
}
