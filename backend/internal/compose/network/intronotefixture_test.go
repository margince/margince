// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package network

// The seam the certification lane reads this site through.
//
// It exists because the cert case lives in `compose` and this site's material
// does not, and the whole point is that a case does NOT rebuild the facts. So
// the test that carries the weight here is the one comparing the seam's facts
// against factsFromRoute's — the assembler the HTTP handler runs. The first
// version of this file compared the seam to the one-line function it wraps,
// which is a tautology, and the divergence it should have caught (the seam
// detected an output language this endpoint never detects) went unseen.

import (
	"strings"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/draftfloor"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

func noteFixture() IntroNoteFixture {
	return IntroNoteFixture{
		Colleague: "Sofia Meier",
		Contact:   "Philipp Königs",
		Requester: "Jonas Weber",
		Through:   "Tobias Reinhardt",
		Band:      "moderate",
		LastAt:    "2026-08-20",
		Value:     "We have done depot retrofits at two carriers their size.",
	}
}

// routeFor is the same scenario as the handler receives it: the graph, the
// route candidate and the request body factsFromRoute reads.
func routeFor(t *testing.T, fixture IntroNoteFixture) noteFacts {
	t.Helper()
	bucket := crmcontracts.PersonGraphRouteCandidateStrengthBucket(fixture.Band)
	route := crmcontracts.PersonGraphRouteCandidate{ViaDisplayName: fixture.Colleague}
	if fixture.Band != "" {
		route.StrengthBucket = &bucket
	}
	if fixture.Through != "" {
		route.ThroughDisplayName = &fixture.Through
	}
	if when, err := time.Parse("2006-01-02", fixture.LastAt); err == nil {
		route.Evidence.LastAt = &when
	}
	body := crmcontracts.DraftIntroNoteJSONRequestBody{}
	if fixture.Value != "" {
		body.ValueForTarget = &fixture.Value
	}
	graph := &crmcontracts.PersonGraph{Nodes: []crmcontracts.PersonGraphNode{{
		Group: crmcontracts.PersonGraphNodeGroupAnchor,
		Label: fixture.Contact,
	}}}
	return factsFromRoute(graph, route, fixture.Requester, body)
}

// A scenario becomes the SAME facts the handler assembles, field for field.
//
// This is the test the seam exists to pass. Equality rather than non-emptiness:
// a seam that swapped the colleague and the contact would fill every field and
// certify a note addressed to the wrong person.
func TestAScenarioBecomesTheFactsTheHandlerAssembles(t *testing.T) {
	t.Parallel()
	fixture := noteFixture()
	seam, err := IntroNoteFactsFor(fixture)
	if err != nil {
		t.Fatalf("building the facts: %v", err)
	}
	handler := routeFor(t, fixture)
	// Field by field rather than struct equality, because lastAt is a POINTER:
	// two assemblies of one date are equal times behind different addresses, so
	// == compares what neither caller means by "the same facts".
	for what, pair := range map[string][2]string{
		"colleague": {seam.colleague, handler.colleague},
		"contact":   {seam.contact, handler.contact},
		"requester": {seam.requester, handler.requester},
		"through":   {seam.through, handler.through},
		"band":      {seam.band, handler.band},
		"value":     {seam.value, handler.value},
		"language":  {string(seam.lang), string(handler.lang)},
	} {
		if pair[0] != pair[1] {
			t.Errorf("the seam assembled %s as %q, and the handler assembles %q", what, pair[0], pair[1])
		}
	}
	if !sameInstant(seam.lastAt, handler.lastAt) {
		t.Errorf("the seam assembled last_spoke as %v, and the handler assembles %v",
			seam.lastAt, handler.lastAt)
	}
	// And the facts are the FIXTURE's, so a seam that agreed with the handler
	// on values neither took from the scenario would still be caught.
	if seam.contact != fixture.Contact || seam.colleague != fixture.Colleague {
		t.Errorf("the facts do not carry the scenario's own people: %+v", seam)
	}
}

// sameInstant compares two optional times by value.
func sameInstant(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Equal(*b)
}

// The output language is the writer's default, NOT a guess from the scenario.
//
// This endpoint sets textlang.Unknown and lets noteLang default it; org360's
// sibling detects a language from the contact's correspondence. Copying that
// detection into this seam is what the first version did, and it would have
// certified a German prompt this product cannot send.
func TestTheSeamLeavesTheLanguageToTheWriterAsTheHandlerDoes(t *testing.T) {
	t.Parallel()
	facts, err := IntroNoteFactsFor(noteFixture())
	if err != nil {
		t.Fatalf("building the facts: %v", err)
	}
	if facts.lang != textlang.Unknown {
		t.Fatalf("the seam decided the language was %q; this endpoint decides nothing and "+
			"lets the writer default it", facts.lang)
	}
	if got := noteLang(facts.lang); got != draftfloor.DefaultLang {
		t.Fatalf("the writer defaulted to %q, want the shared default", got)
	}
}

// A band from the wrong contract's vocabulary is refused rather than sent.
//
// The route's buckets are none/weak/moderate/strong; the company page's are
// cold/developing/strong. A scenario in the wrong one puts a word in the prompt
// that this endpoint never sends, so every rubric scoring against it grades a
// prompt nobody runs — and the run looks clean.
func TestABandFromTheOtherContractsVocabularyIsRefused(t *testing.T) {
	t.Parallel()
	for _, wrong := range []string{"developing", "cold", "warm", "close"} {
		fixture := noteFixture()
		fixture.Band = wrong
		if _, err := IntroNoteFactsFor(fixture); err == nil {
			t.Errorf("%q was accepted as a route strength bucket", wrong)
		}
	}
	// And a route with no recorded strength is ordinary, not an error: the note
	// then says nothing about how warm the relationship is.
	unscored := noteFixture()
	unscored.Band = ""
	facts, err := IntroNoteFactsFor(unscored)
	if err != nil {
		t.Fatalf("a route with no recorded strength was refused: %v", err)
	}
	if facts.band != "" {
		t.Errorf("an unscored route reached the facts as %q", facts.band)
	}
}

// An unreadable date is read as NO date, which is the safe direction: the note
// then says the two are in touch and stops, rather than printing something that
// is not a date into a message a customer reads.
func TestAnUnreadableDateIsReadAsNoDate(t *testing.T) {
	t.Parallel()
	for _, spelling := range []string{"", "last August", "2026-13-45"} {
		broken := noteFixture()
		broken.LastAt = spelling
		facts, err := IntroNoteFactsFor(broken)
		if err != nil {
			t.Fatalf("%q was refused rather than read as no date: %v", spelling, err)
		}
		if facts.lastAt != nil {
			t.Errorf("%q was read as the date %v", spelling, facts.lastAt)
		}
		if got := noteLastSpoke(facts); got != "not recorded" {
			t.Errorf("the prompt would carry %q for an unreadable date", got)
		}
	}
}

// A readable date reaches the prompt in UTC. Read in local time it would shift
// a "last spoke" by a day for half the world.
func TestAReadableDateReachesThePromptInUTC(t *testing.T) {
	t.Parallel()
	facts, err := IntroNoteFactsFor(noteFixture())
	if err != nil {
		t.Fatalf("building the facts: %v", err)
	}
	if want := time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC); !facts.lastAt.Equal(want) {
		t.Fatalf("last_spoke read as %v, want %v", facts.lastAt, want)
	}
	if got := noteLastSpoke(facts); got != "2026-08-20" {
		t.Fatalf("the prompt would carry %q, want 2026-08-20", got)
	}
}

// The request the seam builds carries the scenario's facts as fenced DATA, and
// refuses a scenario the endpoint could not be handed.
func TestTheSeamsRequestCarriesTheFactsAndRefusesABadScenario(t *testing.T) {
	t.Parallel()
	fixture := noteFixture()
	req, err := IntroNoteRequestFor(fixture)
	if err != nil {
		t.Fatalf("building the request: %v", err)
	}
	for _, fact := range []string{fixture.Contact, fixture.Colleague, fixture.Requester, fixture.Value} {
		if !strings.Contains(req.Messages[0].Content, fact) {
			t.Errorf("%q never reached the model", fact)
		}
	}
	if req.SecretStripper == nil {
		t.Error("the seam built the call without the endpoint's secret stripper")
	}
	bad := noteFixture()
	bad.Band = "developing"
	if _, err := IntroNoteRequestFor(bad); err == nil {
		t.Error("a request was built from a scenario the endpoint could not be handed")
	}
	// And the CHECK refuses it too, rather than reading a reply against facts it
	// could not assemble: a scenario that got past one and not the other would
	// grade a real reply against an empty fixture.
	if _, _, err := CheckIntroNote(`{"subject":"x","body":"y"}`, bad); err == nil {
		t.Error("a reply was checked against a scenario the endpoint could not be handed")
	}
}

// The seam checks a reply the way the endpoint checks it, refusals included.
func TestTheSeamChecksAReplyTheWayTheEndpointDoes(t *testing.T) {
	t.Parallel()
	fixture := noteFixture()
	sendable := `{"subject":"Introducing Jonas Weber","body":"Hi Philipp,\n\nMeet Jonas Weber.\n\nSofia"}`

	subject, body, err := CheckIntroNote(sendable, fixture)
	if err != nil {
		t.Fatalf("a sendable note was refused: %v", err)
	}
	if subject == "" || body == "" {
		t.Fatal("the seam returned an empty note it had just accepted")
	}
	// The refusals are the endpoint's: no subject, and a body that never names
	// the rep the note is about.
	for what, reply := range map[string]string{
		"no subject":    `{"subject":"","body":"Hi Philipp,\n\nMeet Jonas Weber."}`,
		"names nobody":  `{"subject":"An introduction","body":"Hi Philipp,\n\nMeet a colleague."}`,
		"greets nobody": `{"subject":"Introducing Jonas Weber","body":"Hello,\n\nMeet Jonas Weber."}`,
		"not the shape": `here you go`,
	} {
		if _, _, err := CheckIntroNote(reply, fixture); err == nil {
			t.Errorf("the seam accepted a reply the endpoint refuses (%s)", what)
		}
	}
}
