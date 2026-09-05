// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package webread

// What a client-rendered page still tells a reader that runs no JavaScript.
//
// The case behind these: a Next.js marketing site serves a shell with no body
// text, so the read judged it "no readable text" and settled the domain as
// parked — a real company on file as an empty address. The words were in the
// markup the whole time, in the schema.org block the framework emitted.

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
)

func ldPage(block string) string {
	return `<html><head><script type="application/ld+json">` + block +
		`</script></head><body><div id="__next"></div></body></html>`
}

func TestAShellDeclaringAnOrganizationIsNotAnEmptyPage(t *testing.T) {
	got := linkedDataClaims(ldPage(`{
		"@context":"https://schema.org","@type":"Organization",
		"name":"Intouch Sports","description":"Coaching for teams that travel."}`))

	want := []string{"Intouch Sports", "Coaching for teams that travel."}
	if !slices.Equal(got, want) {
		t.Errorf("claims = %q, want %q", got, want)
	}
}

// The type is deliberately unfiltered, and this is the case that decides it:
// schema.org's organization vocabulary is open, so a list of accepted types
// would read a site declaring itself a LocalBusiness subtype as declaring
// nothing at all.
func TestAnOrganizationSubtypeIsReadLikeAnyOther(t *testing.T) {
	for _, kind := range []string{"Corporation", "NGO", "Dentist", "SportsActivityLocation"} {
		got := linkedDataClaims(ldPage(fmt.Sprintf(
			`{"@context":"https://schema.org","@type":%q,"name":"Nordic Works"}`, kind)))
		if len(got) != 1 || got[0] != "Nordic Works" {
			t.Errorf("@type %s: claims = %q, want the declared name", kind, got)
		}
	}
}

// A @graph, which is how most frameworks emit more than one node.
func TestAGraphIsWalkedAndTheNamesDeduped(t *testing.T) {
	got := linkedDataClaims(ldPage(`{"@context":"https://schema.org","@graph":[
		{"@type":"WebSite","name":"Nordic Works"},
		{"@type":"Organization","name":"Nordic Works","description":"We build boats."}]}`))

	want := []string{"Nordic Works", "We build boats."}
	if !slices.Equal(got, want) {
		t.Errorf("claims = %q, want the name once and the description after it", got)
	}
}

// The bound is the claim count, because that is the one that cannot fail short.
func TestACatalogueIsCutToTheClaimBound(t *testing.T) {
	var nodes []string
	for i := range 20 {
		nodes = append(nodes, fmt.Sprintf(`{"@type":"Product","name":"Product %02d"}`, i))
	}
	got := linkedDataClaims(ldPage(`[` + strings.Join(nodes, ",") + `]`))

	if len(got) != ldMaxClaims {
		t.Errorf("claims = %d, want the bound of %d", len(got), ldMaxClaims)
	}
	// In document order, so one page reads the same way twice.
	if got[0] != "Product 00" {
		t.Errorf("first claim = %q, want the first node's", got[0])
	}
}

// A page's own broken JSON is not this crawl's problem to report: the block is
// one source of prose among several, and the head and the body still read.
func TestMalformedLinkedDataIsSkippedRatherThanFailing(t *testing.T) {
	got := linkedDataClaims(ldPage(`{"@type":"Organization","name":`))
	if len(got) != 0 {
		t.Errorf("claims = %q, want none from an unparseable block", got)
	}
}

// The whole page, through the fetch a caller actually makes.
//
// Driven end to end rather than by calling the reader directly: what the ticket
// is about is a FETCHED page reading as empty, and a test that assembled the
// claims itself would pass over a Page that never carried them.
func TestAFetchedShellCarriesWhatItDeclared(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><head>` +
			`<meta name="description" content="Coaching for teams that travel.">` +
			`<script type="application/ld+json">{"@type":"Organization","name":"Intouch Sports"}</script>` +
			`<script type="module" src="/_next/main.js"></script>` +
			`</head><body><div id="__next"></div></body></html>`))
	}))
	defer server.Close()

	page, err := newFetcher(server.Client().Transport).FetchPage(context.Background(), server.URL+"/")
	if err != nil {
		t.Fatalf("FetchPage: %v", err)
	}

	// The shape the defect is about: a body with nothing in it.
	if strings.TrimSpace(page.Text) != "" {
		t.Fatalf("page text = %q, want the empty body this case is about", page.Text)
	}
	if !slices.Contains(page.HeadText, "Intouch Sports") {
		t.Errorf("head text = %q, want it to carry the organization the markup declared — "+
			"without it this page reads as having no readable text and settles as parked",
			page.HeadText)
	}
	// And the head's own prose still leads: it is written for a stranger, which
	// is the question both model lanes ask.
	if len(page.HeadText) == 0 || page.HeadText[0] != "Coaching for teams that travel." {
		t.Errorf("head text = %q, want the meta description first", page.HeadText)
	}
}
