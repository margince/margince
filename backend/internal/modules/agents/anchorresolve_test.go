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
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// rankedHits is an engine page of these refs, in the order given.
func rankedHits(refs ...datasource.EntityRef) *rankingProbe {
	page := RecordSearchPage{}
	for _, ref := range refs {
		page.Hits = append(page.Hits, RecordSearchHit{Ref: ref})
	}
	return &rankingProbe{page: page}
}

// aCompany is a hit of the type every case here anchors on.
func aCompany() datasource.EntityRef {
	return datasource.EntityRef{Type: "company", ID: ids.NewV7()}
}

// requireBadArgs asserts the refusal is the actionable kind.
//
// BadArgsError is what carries the caller's own words back to them; anything
// else reaches an agent as this program's prose, which it can neither act on
// nor is entitled to read.
func requireBadArgs(t *testing.T, err error) {
	t.Helper()
	if _, ok := errors.AsType[*BadArgsError](err); !ok {
		t.Fatalf("err = %v, want a BadArgsError a host can act on", err)
	}
}

// refuseAnchor resolves as a company reader and requires a refusal.
func refuseAnchor(t *testing.T, engine *rankingProbe, args anchorArgs) error {
	t.Helper()
	got, err := resolveAnchor(searchReader("company"), engine.search, nil, args)
	requireBadArgs(t, err)
	if got != (ids.UUID{}) {
		t.Errorf("returned id %s beside a refusal", got)
	}
	return err
}

// resolvesTo resolves as a company reader and requires the id it should find.
func resolvesTo(t *testing.T, engine *rankingProbe, args anchorArgs, want ids.UUID) {
	t.Helper()
	got, err := resolveAnchor(searchReader("company"), engine.search, nil, args)
	if err != nil {
		t.Fatalf("resolveAnchor: %v", err)
	}
	if got != want {
		t.Fatalf("resolved to %s, want %s", got, want)
	}
}

// An id given outright is used as given: naming the record precisely must not
// cost a search.
func TestAnAnchorGivenAnIDDoesNotSearch(t *testing.T) {
	engine := &rankingProbe{}
	want := ids.NewV7()

	resolvesTo(t, engine, anchorArgs{RecordType: "company", RecordID: want}, want)

	if engine.asked != nil {
		t.Errorf("searched for %q when the caller had already named the record", engine.asked.Text)
	}
}

// One match is the record. The name is ranked by the engine search_records
// runs, scoped to the type the caller named.
func TestAnAnchorNamedInWordsResolvesToItsOneMatch(t *testing.T) {
	hit := aCompany()
	engine := rankedHits(hit)

	resolvesTo(t, engine, anchorArgs{RecordType: "company", RecordName: "Contoso"}, hit.ID)

	want := RecordSearchQuery{Text: "Contoso", Types: []string{"company"}, Limit: anchorResolveLimit}
	if got := engine.asked; got == nil || got.Text != want.Text || !slices.Equal(got.Types, want.Types) ||
		got.Limit != want.Limit || got.PerType != nil || got.WithEmployees || got.Cursor != "" {
		t.Errorf("the engine was asked %+v, want %+v", got, want)
	}
}

// Several matches are REFUSED with the candidates, never guessed. The top hit
// is not the record the caller meant just because it ranked first. The
// candidates are listed in rank order.
func TestAnAmbiguousNameIsRefusedWithItsCandidates(t *testing.T) {
	first, second := aCompany(), aCompany()

	err := refuseAnchor(t, rankedHits(first, second), anchorArgs{RecordType: "company", RecordName: "Contoso"})

	if !strings.Contains(err.Error(), first.ID.String()+", "+second.ID.String()) {
		t.Errorf("the refusal does not name both candidates in rank order: %v", err)
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
	err := refuseAnchor(t, &rankingProbe{}, anchorArgs{RecordType: "company", RecordName: "Nobody"})

	// It quotes the name that found nothing, because the caller's next move is
	// to try a different one and they need to know which was tried.
	if !strings.Contains(err.Error(), "Nobody") {
		t.Errorf("the refusal does not name what was searched for: %v", err)
	}
	// search_records ranks the same words on the same engine, so it is where
	// the caller sees what the words do find.
	if !strings.Contains(err.Error(), "search_records") {
		t.Errorf("the refusal does not send the caller to search_records: %v", err)
	}
}

// The engine leaves a type the caller may not read out of its answer. Naming
// that type is a denial, not "nothing matches".
func TestANameOfATypeTheCallerCannotReadIsDenied(t *testing.T) {
	engine := rankedHits(aCompany())
	_, err := resolveAnchor(searchReader("contact"), engine.search, nil, anchorArgs{RecordType: "company", RecordName: "Contoso"})
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("naming an unreadable type answered %v, want ErrPermissionDenied", err)
	}
	if engine.asked != nil {
		t.Error("the engine ran for a type the caller cannot read")
	}
}

// The candidate list carries ids and nothing else. The refusal travels to an
// agent that may hold no grant on any of them. A snippet would publish what the
// record says to somebody who asked only whether a word was ambiguous.
func TestTheCandidateListPublishesNoRecordContent(t *testing.T) {
	engine := rankedHits(aCompany(), aCompany())
	engine.page.Hits[0].Match = RecordMatch{Snippet: "Contoso Pharmaceuticals"}

	err := refuseAnchor(t, engine, anchorArgs{RecordType: "company", RecordName: "Contoso"})

	if strings.Contains(err.Error(), "Pharmaceuticals") {
		t.Errorf("the refusal published record content: %v", err)
	}
}

// Naming a record both ways is refused rather than resolved from the id: a
// request carrying both is one whose author believed they agreed, and answering
// from the id hides the disagreement on the call where it could still be seen.
func TestNamingARecordBothWaysIsRefused(t *testing.T) {
	requireBadArgs(t, anchorArgs{
		RecordType: "company", RecordID: ids.NewV7(), RecordName: "Contoso",
	}.validate())
}

// Naming it neither way is refused too. `record_id` left the schema's required
// list when `record_name` arrived, so the surface-wide id check no longer makes
// this claim and this is where it is made.
func TestNamingARecordNoWayIsRefused(t *testing.T) {
	for _, args := range []anchorArgs{
		{RecordType: "company"},
		{RecordType: "company", RecordName: "   "},
	} {
		requireBadArgs(t, args.validate())
	}
}
