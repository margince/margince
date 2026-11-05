// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package webread

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// latin1 encodes s byte for byte as ISO-8859-1; every rune in it must be
// below U+0100.
func latin1(s string) []byte {
	out := make([]byte, 0, len(s))
	for _, r := range s {
		out = append(out, byte(r))
	}
	return out
}

// German imprint pages are still served in Latin-1. The charset may be named
// in the header, in a meta tag or nowhere. In every case the text must reach
// callers as UTF-8 umlauts, not replacement characters.
func TestFetchDecodesLegacyCharsetPagesToUTF8(t *testing.T) {
	cases := []struct {
		name, contentType string
		body              []byte
	}{
		{
			name:        "charset in the Content-Type header",
			contentType: "text/html; charset=iso-8859-1",
			body:        latin1(`<html><body>Geschäftsführer: Dirk Hörig</body></html>`),
		},
		{
			name:        "charset in a meta tag",
			contentType: "text/html",
			body:        latin1(`<html><head><meta charset="windows-1252"></head><body>Geschäftsführer: Dirk Hörig</body></html>`),
		},
		{
			name:        "no charset declared at all",
			contentType: "text/html",
			body:        latin1(`<html><body>Geschäftsführer: Dirk Hörig</body></html>`),
		},
		{
			name:        "a UTF-8 page whose server wrongly claims Latin-1",
			contentType: "text/html; charset=iso-8859-1",
			body:        []byte(`<html><body>Geschäftsführer: Dirk Hörig</body></html>`),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/robots.txt" {
					http.NotFound(w, r)
					return
				}
				w.Header().Set("Content-Type", tc.contentType)
				//craft:ignore swallowed-errors httptest handler write; a failed write fails the test through the assertions below
				_, _ = w.Write(tc.body)
			}))
			defer srv.Close()
			f := testFetcher()

			const want = "Geschäftsführer: Dirk Hörig"
			doc, err := f.Fetch(context.Background(), srv.URL+"/impressum")
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(doc.Text, want) {
				t.Errorf("Fetch text = %q, want it to carry %q", doc.Text, want)
			}
			page, err := f.FetchPage(context.Background(), srv.URL+"/impressum")
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(page.Text, want) {
				t.Errorf("FetchPage text = %q, want it to carry %q", page.Text, want)
			}
			if page.Bytes != len(tc.body) {
				t.Errorf("Page.Bytes = %d, want the raw size %d", page.Bytes, len(tc.body))
			}
		})
	}
}

// The size cap can cut a UTF-8 page inside a multi-byte rune. That cut must
// not make the page look like Latin-1, or every umlaut before it would be
// decoded into two wrong letters. The umlauts sit past the meta-tag window,
// where the charset sniff no longer sees them.
func TestTextKeepsAUTF8BodyCutInsideARune(t *testing.T) {
	const kept = "Dirk Hörig, Geschäftsführer "
	padding := strings.Repeat("<p>Impressum</p>", charsetSniffBytes/8)
	full := []byte(padding + kept + "ö")
	cut := fetched{body: full[:len(full)-1], contentType: "text/html", capped: true}
	if got := cut.text(); got != padding+kept {
		t.Errorf("text() ends in %q, want the UTF-8 body without its cut rune", got[len(padding):])
	}
}

// The fetch itself must report the cut: a UTF-8 page longer than the cap,
// cut inside its last umlaut, still reads as UTF-8.
func TestFetchKeepsAUTF8PageThatOverrunsTheCap(t *testing.T) {
	const kept = "Dirk Hörig "
	padding := strings.Repeat("a", maxFetchBytes-len(kept)-1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		//craft:ignore swallowed-errors httptest handler write; a failed write fails the test through the assertions below
		_, _ = w.Write([]byte(padding + kept + "ö and more"))
	}))
	defer srv.Close()

	doc, err := testFetcher().Fetch(context.Background(), srv.URL+"/")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(doc.Text, "Dirk Hörig") {
		t.Errorf("text ends in %q, want %q", doc.Text[len(doc.Text)-20:], "Dirk Hörig")
	}
}

// Only the cap can cut a rune. A whole body that happens to end in a byte
// shaped like a UTF-8 lead byte ends in a Latin-1 letter.
func TestTextDecodesAnUncappedBodyEndingInALeadByte(t *testing.T) {
	bodies := map[string][]byte{
		"Inhaber: José":        latin1("Inhaber: José"),
		"Inhaber: Hörig, José": append([]byte("Inhaber: Hörig, Jos"), 0xe9),
	}
	for want, body := range bodies {
		if got := (fetched{body: body, contentType: "text/html"}).text(); got != want {
			t.Errorf("text() = %q, want %q", got, want)
		}
	}
}

// At the cap, a trailing lead byte is a cut rune only when the rest is valid
// UTF-8 with an umlaut in it. Otherwise it is a Latin-1 letter.
func TestTextDecodesACappedBodyEndingInALatin1Letter(t *testing.T) {
	for _, want := range []string{"Inhaber: José", "Dirk Hörig, José"} {
		capped := fetched{body: latin1(want), contentType: "text/html", capped: true}
		if got := capped.text(); got != want {
			t.Errorf("text() = %q, want %q", got, want)
		}
	}
}

// A UTF-8 page can carry a Windows-1252 byte past the charset sniff window,
// for example from an included snippet. Both halves must come out readable,
// whatever charset the header names.
func TestTextReadsAUTF8PageWithALateLegacyByte(t *testing.T) {
	body := []byte("Dirk Hörig " + strings.Repeat("a", charsetSniffBytes) + " 2019\x962026")
	for _, contentType := range []string{"text/html", "text/html; charset=utf-8", "text/html; charset=iso-8859-1"} {
		got := fetched{body: body, contentType: contentType}.text()
		if !strings.HasPrefix(got, "Dirk Hörig ") || !strings.HasSuffix(got, " 2019–2026") {
			t.Errorf("text() for %q = %q…%q, want the UTF-8 name and the en dash",
				contentType, got[:12], got[len(got)-12:])
		}
	}
}

// A body that is not text reaches the caller byte for byte. JSON is UTF-8 by
// definition, and a binary body has no charset to decode.
func TestTextLeavesNonTextBodiesUntouched(t *testing.T) {
	body := latin1("Hörig")
	for _, contentType := range []string{"application/json", "text/json", "application/octet-stream", ""} {
		got := fetched{body: body, contentType: contentType}.text()
		if got != string(body) {
			t.Errorf("text() for %q = %q, want the bytes unchanged", contentType, got)
		}
	}
}
