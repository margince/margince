// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"fmt"
	"slices"
	"strings"
	"testing"
)

func TestOnlyGoogleDocsAndDriveAreDocumentHosts(t *testing.T) {
	cases := []struct {
		link string
		ok   bool
	}{
		{"https://docs.google.com/presentation/d/abc/edit", true},
		{"https://DOCS.google.com/document/d/abc", true},
		{"http://drive.google.com/file/d/x/view", true},
		{"https://docs.google.com./document/d/abc", true},
		{"https://docs.google.com.example.net/doc", false},
		{"https://example.net/docs.google.com", false},
		{"https://example.net/?next=https://docs.google.com/a", false},
		{"https://user@docs.google.com/doc", false},
		{"https://user:pw@drive.google.com/file", false},
		{"ftp://docs.google.com/doc", false},
		{"https://www.google.com/search", false},
		{"https://dropbox.com/s/abc", false},
		{"not a url", false},
	}
	for _, tc := range cases {
		t.Run(tc.link, func(t *testing.T) {
			if _, ok := documentLinkKey(tc.link); ok != tc.ok {
				t.Errorf("documentLinkKey(%q) = %v, want %v", tc.link, ok, tc.ok)
			}
		})
	}
}

func TestDocumentLinksOfKeepsDocumentsOnceEach(t *testing.T) {
	links, over := documentLinksOf(&MailEnvelope{Links: []MailLink{
		{URL: "https://example.net/page", Text: "ordinary"},
		{URL: "https://docs.google.com/presentation/d/abc/edit#slide=1", Text: "Deck"},
		{URL: "https://DOCS.google.com/presentation/d/abc/edit#slide=2"},
	}})

	want := []MailLink{{URL: "https://docs.google.com/presentation/d/abc/edit", Text: "Deck"}}
	if !slices.Equal(links, want) || over != 0 {
		t.Errorf("document links are %v (over %d), want %v", links, over, want)
	}
	if none, _ := documentLinksOf(nil); none != nil {
		t.Errorf("a mail with no envelope gave links %v", none)
	}
}

func TestDocumentLinksPerMessageAreCapped(t *testing.T) {
	var all []MailLink
	for i := range maxDocumentLinksPerMessage + 3 {
		all = append(all, MailLink{URL: fmt.Sprintf("https://docs.google.com/document/d/%d", i)})
	}
	links, over := documentLinksOf(&MailEnvelope{Links: all})

	if len(links) != maxDocumentLinksPerMessage || over != 3 {
		t.Errorf("kept %d links, over %d; want %d and 3", len(links), over, maxDocumentLinksPerMessage)
	}
}

func TestDocumentLinkKeyCanonicalizesHostPortAndLength(t *testing.T) {
	want, _ := documentLinkKey("https://docs.google.com/document/d/abc")
	for _, same := range []string{
		"https://docs.google.com./document/d/abc",
		"https://DOCS.google.com:443/document/d/abc#frag",
	} {
		if got, ok := documentLinkKey(same); !ok || got != want {
			t.Errorf("documentLinkKey(%q) = %q, %v; want %q", same, got, ok, want)
		}
	}
	if _, ok := documentLinkKey("https://docs.google.com/d/" + strings.Repeat("a", maxDocumentLinkLength)); ok {
		t.Error("an address over the length bound must be skipped")
	}
}
