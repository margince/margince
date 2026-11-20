// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package webread

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func servePage(t *testing.T, head string) Page {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			http.NotFound(w, r)
			return
		}
		//craft:ignore swallowed-errors httptest handler write; a failed write fails the test through the assertions below
		_, _ = w.Write([]byte("<html><head>" + head + "</head><body>Hello</body></html>"))
	}))
	t.Cleanup(srv.Close)
	page, err := testFetcher().FetchPage(context.Background(), srv.URL+"/")
	if err != nil {
		t.Fatal(err)
	}
	return page
}

func TestAPageIsNamedByItsShareTitleBeforeItsTitleTag(t *testing.T) {
	page := servePage(t, `<title>Pricing | Acme — robots for all</title>
		<meta property="og:title" content="Pricing">
		<meta property="og:site_name" content="Acme">`)
	if page.Title != "Pricing" || page.SiteName != "Acme" {
		t.Fatalf("title, site = %q, %q; want the Open Graph names", page.Title, page.SiteName)
	}
}

func TestATitleTagIsReadOnceAndAsOneLine(t *testing.T) {
	page := servePage(t, "<title>\n  Fish &amp;lt;3 &amp; chips\n</title><title>Second</title>")
	if page.Title != "Fish &lt;3 & chips" {
		t.Fatalf("title = %q, want the first tag's text unescaped once, on one line", page.Title)
	}
	if page.SiteName != "" {
		t.Fatalf("site = %q, want none named", page.SiteName)
	}
}
