// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

func TestNormalizeLinkedInURLProducesOneSpellingPerProfile(t *testing.T) {
	// Every spelling of the same profile must reduce to the same key —
	// that identity IS the E12.11 exact-match dedupe.
	cases := map[string]string{
		"https://www.linkedin.com/in/vera-vp":                  "https://www.linkedin.com/in/vera-vp",
		"https://WWW.LinkedIn.com/in/vera-vp":                  "https://www.linkedin.com/in/vera-vp",
		"https://www.linkedin.com/in/vera-vp/":                 "https://www.linkedin.com/in/vera-vp",
		"https://www.linkedin.com/in/vera-vp?utm_source=share": "https://www.linkedin.com/in/vera-vp",
		"https://www.linkedin.com/in/vera-vp#about":            "https://www.linkedin.com/in/vera-vp",
		"http://www.linkedin.com/in/vera-vp":                   "https://www.linkedin.com/in/vera-vp",
		"www.linkedin.com/in/vera-vp":                          "https://www.linkedin.com/in/vera-vp",
		"  https://www.linkedin.com/in/vera-vp/?trk=profile  ": "https://www.linkedin.com/in/vera-vp",
		"https://www.linkedin.com:443/in/vera-vp":              "https://www.linkedin.com/in/vera-vp",
		"https://de.linkedin.com/in/vera-vp":                   "https://de.linkedin.com/in/vera-vp",
		"https://www.linkedin.com/in/Vera-VP":                  "https://www.linkedin.com/in/Vera-VP", // the slug is identity; only the host case-folds
		// A crawl reads a link off a page that wrote it protocol-relative. It
		// names the same profile as the full form and used to be refused.
		"//www.linkedin.com/in/vera-vp": "https://www.linkedin.com/in/vera-vp",
	}
	for raw, want := range cases {
		got, err := NormalizeLinkedInURL(raw)
		if err != nil {
			t.Errorf("NormalizeLinkedInURL(%q): %v", raw, err)
			continue
		}
		if got != want {
			t.Errorf("NormalizeLinkedInURL(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestNormalizeLinkedInURLRefusesNonURLs(t *testing.T) {
	// "https://" keeps its place beside the protocol-relative case above: both
	// parse to an empty hostname, and only one of them names a profile.
	for _, raw := range []string{"", "   ", "ftp://linkedin.com/in/x", "https://", "://nope", "//"} {
		_, err := NormalizeLinkedInURL(raw)
		var parseErr *values.ParseError
		if !errors.As(err, &parseErr) {
			t.Errorf("NormalizeLinkedInURL(%q): got %v, want a values.ParseError", raw, err)
		}
	}
}

// The narrow pending read refuses the zero contact rather than answering the
// wide list. The two reads share one gated query, and ids.Nil is exactly what
// the unfiltered entry point passes it — so a caller that meant to name one
// contact and computed the zero id would silently be handed every open match
// the member has, staged as though each were about the arrival that triggered
// the pass. The refusal comes before any pool use, which is why a nil store
// answers it.
func TestAPersonScopedPendingMatchReadRefusesTheZeroContact(t *testing.T) {
	_, err := NewStore(nil).PendingLinkedInMatchesForPerson(context.Background(), ids.Nil)
	if err == nil {
		t.Fatal("the zero contact was accepted — a person-scoped read that names no person " +
			"silently widens to every pending match the member has")
	}
}

// The classifier decides LinkedIn from website for a vCard's single URL and for
// a card reader's, and its answer has to be one the storing writer will honour:
// a value classified as a profile and then refused by the normalizer is a card
// whose URL reaches neither slot.
func TestIsLinkedinURLAnswersForTheValueThatWouldBeStored(t *testing.T) {
	profiles := []string{
		"https://www.linkedin.com/in/vera-vp",
		"https://de.linkedin.com/in/vera-vp",
		"linkedin.com/in/vera-vp",
		"//www.linkedin.com/in/vera-vp",
		"https://LINKEDIN.com/in/vera-vp",
	}
	for _, raw := range profiles {
		if !isLinkedinURL(raw) {
			t.Errorf("isLinkedinURL(%q) = false, want true", raw)
		}
	}
	others := []string{
		"https://vera.example/team/linkedin",
		"https://notlinkedin.com/in/vera-vp",
		// A scheme the writer refuses. It used to classify as a profile on the
		// strength of its host and then be dropped two calls later, so the card
		// reader filed it as LinkedIn and the record gained neither.
		"javascript://linkedin.com/in/vera-vp",
		// The shortener the CLIENT will draw under the word LinkedIn and this
		// writer will not store — LinkedInDisplayOnlyHosts says why.
		"https://lnkd.in/abcdef",
		"",
	}
	for _, raw := range others {
		if isLinkedinURL(raw) {
			t.Errorf("isLinkedinURL(%q) = true, want false", raw)
		}
	}
}

// The two declared halves are what the client's array is held to, so a host in
// both would make that comparison pass while the writer and the renderer
// disagreed about which of them owns it.
func TestTheDeclaredLinkedInHostHalvesDoNotOverlap(t *testing.T) {
	storable := map[string]bool{}
	for _, host := range LinkedInSlotHosts() {
		storable[host] = true
	}
	for _, host := range LinkedInDisplayOnlyHosts() {
		if storable[host] {
			t.Errorf("%q is declared both storable and display-only", host)
		}
	}
}
