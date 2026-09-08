// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package webread

// What a page calls its logo, and in which order it says so.

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"
)

func acmeBase(t *testing.T) *url.URL {
	t.Helper()
	base, err := url.Parse("https://acme.example/de/")
	if err != nil {
		t.Fatal(err)
	}
	return base
}

func TestTheJSONLDLogoLeadsAndTheLabelledImagesFollow(t *testing.T) {
	// The schema.org block says "logo" in a vocabulary made for the purpose;
	// the header <img> says it in its alt text. Both count, the explicit one
	// first, and a relative address resolves against where the page came from.
	page := `<html><head>
		<script type="application/ld+json">{"@type":"Organization","name":"Acme","logo":"/brand/acme-lockup.png"}</script>
		</head><body>
		<header><a href="/"><img src="../img/header.svg" alt="Acme Logo"></a></header>
		<main><img src="/img/hero.jpg" alt="Our team at work"></main>
		</body></html>`
	got := declaredLogos(page, acmeBase(t))
	want := []string{"https://acme.example/brand/acme-lockup.png", "https://acme.example/img/header.svg"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("declared logos:\n got %v\nwant %v", got, want)
	}
}

func TestAnImageIsALogoByAnyOfTheLabelsAPagePutsOnIt(t *testing.T) {
	// alt, class, id and the file name are all places a page labels its mark,
	// and a hero photo labelled none of those ways is not one.
	page := `<body>
		<img src="/a.png" class="site-logo">
		<img src="/b.png" id="Logo_Header">
		<img src="/assets/acme-logo.svg">
		<img src="/hero.jpg" alt="Team" class="hero">
		</body>`
	got := declaredLogos(page, acmeBase(t))
	want := []string{"https://acme.example/a.png", "https://acme.example/b.png", "https://acme.example/assets/acme-logo.svg"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("labelled logos:\n got %v\nwant %v", got, want)
	}
}

func TestDeclaredLogosAreBoundedAndDeduplicated(t *testing.T) {
	// A page that repeats its mark on every card offers it once, and a page
	// with a strip of partner logos offers the first few and not the fortieth.
	var page string
	for i := range 40 {
		page += fmt.Sprintf(`<img src="/logo-%d.png" alt="partner logo">`, i%2)
		page += `<img src="/logo-0.png" alt="partner logo">`
	}
	got := declaredLogos(page, acmeBase(t))
	if len(got) != 2 {
		t.Fatalf("two distinct logos declared, got %v", got)
	}
	var unbounded string
	for i := range 40 {
		unbounded += fmt.Sprintf(`<img src="/logo-%d.png" alt="logo">`, i)
	}
	if got := declaredLogos(unbounded, acmeBase(t)); len(got) != maxDeclaredLogos {
		t.Fatalf("forty distinct logos declared, want the bound of %d, got %d", maxDeclaredLogos, len(got))
	}
}

func TestALogoNeedsAnAddressWorthFetching(t *testing.T) {
	// A data: URI is the picture inline, not an asset; a lazy-loading <img>
	// whose src names nothing is a placeholder. Neither is a candidate.
	page := `<body>
		<img src="data:image/png;base64,iVBORw0KGgo=" alt="logo">
		<img data-src="/real-logo.png" alt="logo">
		<img src="" alt="logo">
		</body>`
	if got := declaredLogos(page, acmeBase(t)); len(got) != 0 {
		t.Fatalf("nothing fetchable was declared, got %v", got)
	}
}

func TestAJSONLDLogoMayBeAnImageObjectOrAList(t *testing.T) {
	// schema.org lets a logo be a URL, an ImageObject, or several; the reader
	// takes each spelling, and a repeated address once.
	page := ldPage(`[{"@type":"Organization","logo":{"@type":"ImageObject","url":"https://cdn.acme.example/lockup.png"}},
		{"@type":"WebSite","publisher":{"@type":"Organization","logo":["https://cdn.acme.example/lockup.png", {"contentUrl":"/second.png"}]}}]`)
	got := declaredLogos(page, acmeBase(t))
	want := []string{"https://cdn.acme.example/lockup.png", "https://acme.example/second.png"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("JSON-LD logos:\n got %v\nwant %v", got, want)
	}
}

func TestAFetchedPageCarriesTheLogosItDeclared(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			http.NotFound(w, r)
			return
		}
		//craft:ignore swallowed-errors httptest handler write; a failed write fails the test through the assertions below
		_, _ = w.Write([]byte(`<html><head><link rel="icon" href="/favicon.png"></head>` +
			`<body><header><img src="/brand/lockup.svg" alt="Acme logo"></header>Acme builds robots.</body></html>`))
	}))
	defer srv.Close()

	page, err := testFetcher().FetchPage(context.Background(), srv.URL+"/")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{srv.URL + "/brand/lockup.svg"}; !reflect.DeepEqual(page.Logos, want) {
		t.Fatalf("Page.Logos = %v, want %v", page.Logos, want)
	}
	// The head's icon is still an icon, not a logo: the two lists answer
	// different questions.
	if len(page.Icons) != 1 || page.Icons[0].URL != srv.URL+"/favicon.png" {
		t.Fatalf("Page.Icons = %v, want the declared favicon alone", page.Icons)
	}
}
