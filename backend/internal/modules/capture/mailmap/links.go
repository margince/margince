// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mailmap

// Where a mail's web links lead. Capture compares these hosts with the
// sender's domain. That tells a link to a third site from one to the sender's
// own website.

import (
	"net/url"
	"regexp"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// maxLinkHosts bounds how many distinct hosts one message reports. One
// unrelated host is enough for the judgement, so a longer list buys nothing.
const maxLinkHosts = 20

// maxLinkTokens bounds the HTML walk. The body is a stranger's text, and the
// walk must not cost more than the rendering it sits beside.
const maxLinkTokens = 20000

// plainLink is a web address written out in plain text. It stops at the
// characters that end an address in running prose.
var plainLink = regexp.MustCompile(`(?i)https?://[^\s<>"'()\[\]]+`)

// linkHosts lists the hosts the body's web links lead to, lowercased and in
// first-seen order. Both bodies are read. A sender can put one link in the
// HTML part and another in the plain part a mail client may never show.
func linkHosts(plain, htmlBody string) []string {
	seen := map[string]bool{}
	var hosts []string
	add := func(raw string) {
		host := linkTarget(raw)
		if host == "" || seen[host] || len(hosts) >= maxLinkHosts {
			return
		}
		seen[host] = true
		hosts = append(hosts, host)
	}
	for _, raw := range plainLink.FindAllString(plain, -1) {
		add(raw)
	}
	for _, raw := range anchorHrefs(htmlBody) {
		add(raw)
	}
	return hosts
}

// anchorHrefs returns the href of each <a> in an HTML body, within the walk bound.
func anchorHrefs(src string) []string {
	if src == "" {
		return nil
	}
	z := html.NewTokenizer(strings.NewReader(src))
	z.SetMaxBuf(maxTokenLen)
	var hrefs []string
	for range maxLinkTokens {
		switch z.Next() {
		case html.ErrorToken:
			return hrefs
		case html.StartTagToken, html.SelfClosingTagToken:
			token := z.Token()
			if token.DataAtom == atom.A {
				hrefs = append(hrefs, attrValue(token, "href"))
			}
		}
	}
	return hrefs
}

// linkTarget is the host a web link really leads to, or "" for anything that
// is not an http or https address. A known redirect wrapper is unwrapped first,
// because its own host says nothing about where the reader lands.
func linkTarget(raw string) string {
	target, ok := webURL(raw)
	if !ok {
		return ""
	}
	if inner, wrapped := unwrapRedirect(target); wrapped {
		if unwrapped, ok := webURL(inner); ok {
			target = unwrapped
		}
	}
	return strings.TrimSuffix(strings.ToLower(target.Hostname()), ".")
}

func webURL(raw string) (*url.URL, bool) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Hostname() == "" {
		return nil, false
	}
	scheme := strings.ToLower(parsed.Scheme)
	return parsed, scheme == "http" || scheme == "https"
}

// unwrapRedirect reads the destination out of Google's link redirect, the
// wrapper calendar and document notices put around every link. Any other
// host is left as it is: guessing at a wrapper's parameter would let a sender
// choose which host the judgement reads.
func unwrapRedirect(target *url.URL) (string, bool) {
	host := strings.TrimPrefix(strings.ToLower(target.Hostname()), "www.")
	if host != "google.com" || target.Path != "/url" {
		return "", false
	}
	query := target.Query()
	for _, key := range []string{"q", "url"} {
		if inner := query.Get(key); inner != "" {
			return inner, true
		}
	}
	return "", false
}
