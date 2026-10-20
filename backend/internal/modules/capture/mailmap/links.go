// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mailmap

// Where a mail's web links lead. Capture compares these hosts with the
// sender's domain. That tells a link to a third site from one to the sender's
// own website.

import (
	"errors"
	"io"
	"net/url"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

// maxLinkHosts bounds how many distinct hosts one message reports. Past it the
// list is marked cut short, so capture still decides.
const maxLinkHosts = 200

// The link walk's bounds. The body is a stranger's text, and the HTML5 parser
// spends time per tag in proportion to how deeply it nests. The parser itself
// refuses more than 512 open elements; the walk counts tags first, and a body
// past either bound is not parsed at all.
const (
	maxLinkHTMLBytes = 1 << 20
	maxLinkTokens    = 20000
)

// maxRedirectDepth bounds how many redirect wrappers one link may nest.
const maxRedirectDepth = 5

// plainLink is a web address written out in plain text. It stops at the
// characters that end an address in running prose.
var plainLink = regexp.MustCompile(`(?i)https?://[^\s<>"'()\[\]]+`)

// linkHosts lists the hosts the body's web links lead to, lowercased and in
// first-seen order. Both bodies are read. A sender can put one link in the
// HTML part and another in the plain part a mail client may never show.
// truncated says some link was not read to its end.
func linkHosts(plain, htmlBody string) (hosts []string, truncated bool) {
	seen := map[string]bool{}
	add := func(raw string) {
		host, settled := linkTarget(raw)
		switch {
		case !settled:
			truncated = true
		case host == "" || seen[host]:
		case len(hosts) >= maxLinkHosts:
			truncated = true
		default:
			seen[host] = true
			hosts = append(hosts, host)
		}
	}
	for _, raw := range plainLink.FindAllString(plain, -1) {
		add(raw)
	}
	hrefs, complete := anchorHrefs(htmlBody)
	for _, raw := range hrefs {
		add(raw)
	}
	return hosts, truncated || !complete
}

// anchorHrefs returns the href of each link element in an HTML body, and
// whether the walk read the whole body. It uses the HTML5 parser with
// scripting off, because that is how a mail client renders. A token walk
// reads <svg><style>, script escapes and <noscript> differently, and a sender
// can hide a link in that gap.
func anchorHrefs(src string) ([]string, bool) {
	if len(src) > maxLinkHTMLBytes || !withinTagBound(src) {
		return nil, false
	}
	doc, err := html.ParseWithOptions(strings.NewReader(src), html.ParseOptionEnableScripting(false))
	if err != nil {
		return nil, false
	}
	var hrefs []string
	for node := range doc.Descendants() {
		if node.Type != html.ElementNode || (node.Data != "a" && node.Data != "area") {
			continue
		}
		for _, attr := range node.Attr {
			if attr.Key == "href" {
				hrefs = append(hrefs, attr.Val)
			}
		}
	}
	return hrefs, true
}

// withinTagBound reports whether a linear token walk of src ends within
// maxLinkTokens.
func withinTagBound(src string) bool {
	z := html.NewTokenizer(strings.NewReader(src))
	for range maxLinkTokens {
		if z.Next() == html.ErrorToken {
			return errors.Is(z.Err(), io.EOF)
		}
	}
	return false
}

// linkTarget is the host a web link really leads to, or "" for anything that
// is not an http or https address. Known redirect wrappers are unwrapped until
// none is left, because a wrapper's host says nothing about where the reader
// lands. settled is false when a wrapper names no web address or the chain is
// deeper than maxRedirectDepth. Each target is shorter than its wrapper, so a
// chain cannot repeat; the depth limit is the only stop needed.
func linkTarget(raw string) (host string, settled bool) {
	target, ok := webURL(raw)
	if !ok {
		return "", true
	}
	for range maxRedirectDepth {
		inner, wrapped := unwrapRedirect(target)
		if !wrapped {
			return strings.TrimSuffix(strings.ToLower(target.Hostname()), "."), true
		}
		if target, ok = webURL(inner); !ok {
			return "", false
		}
	}
	return "", false
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
