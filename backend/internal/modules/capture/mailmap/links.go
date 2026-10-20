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
	"golang.org/x/net/html/atom"
)

// maxLinkHosts bounds how many distinct hosts one message reports. Past it the
// list is marked cut short, so capture still decides.
const maxLinkHosts = 200

// maxLinkTokens bounds the HTML walk. The body is a stranger's text, and the
// walk must not cost more than the rendering it sits beside.
const maxLinkTokens = 20000

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

// anchorHrefs returns the href of each <a> in an HTML body, and whether the
// walk reached the end of the body within its bound.
func anchorHrefs(src string) ([]string, bool) {
	walk := &anchorWalk{}
	for {
		rest, complete, resume := walk.scan(src)
		if !resume {
			return walk.hrefs, complete
		}
		src = rest
	}
}

// anchorWalk is one body's walk, carried across the restarts after a raw-text
// element too long for the tokenizer's buffer.
type anchorWalk struct {
	hrefs  []string
	tokens int
}

// scan walks src until it ends, the token bound runs out, or a <style> or
// <script> holds more than the buffer takes. That content can hold no link.
// So in that one case scan returns the source from the element's end tag on,
// and asks to resume there.
func (w *anchorWalk) scan(src string) (rest string, complete, resume bool) {
	z := html.NewTokenizer(strings.NewReader(src))
	z.SetMaxBuf(maxTokenLen)
	// rawFrom is where the content of an open <style> or <script> starts. The
	// tokenizer hands that content over as text, cut off where the buffer ran
	// out, possibly inside its end tag.
	read, rawFrom, rawTag := 0, -1, ""
	for ; w.tokens < maxLinkTokens; w.tokens++ {
		kind := z.Next()
		if kind == html.ErrorToken {
			if rawFrom >= 0 && errors.Is(z.Err(), html.ErrBufferExceeded) {
				return pastRawText(src[rawFrom:], rawTag)
			}
			// Any other token past the buffer cap also ends here, before the body does.
			return "", errors.Is(z.Err(), io.EOF), false
		}
		read += len(z.Raw())
		if kind == html.TextToken {
			continue
		}
		rawFrom = -1
		if kind != html.StartTagToken && kind != html.SelfClosingTagToken {
			continue
		}
		switch token := z.Token(); token.DataAtom {
		case atom.A:
			w.hrefs = append(w.hrefs, attrValue(token, "href"))
		case atom.Style, atom.Script:
			rawFrom, rawTag = read, token.Data
		case atom.Noscript:
			// A mail client renders with scripting off, so the reader sees
			// this content as markup, links included.
			z.NextIsNotRawText()
		}
	}
	return "", false, false
}

// pastRawText returns the source from the end tag that closes a raw-text
// element. With no end tag the element runs to the end of the body, and so
// does the walk. Any "</style" counts as that end tag, even one the browser
// would read as text, so the walk can only read more markup as links.
func pastRawText(content, tag string) (rest string, complete, resume bool) {
	for from := 0; ; {
		at := strings.Index(content[from:], "</")
		if at < 0 {
			return "", true, false
		}
		at += from
		if name := content[at+2:]; len(name) >= len(tag) && strings.EqualFold(name[:len(tag)], tag) {
			return content[at:], false, true
		}
		from = at + 2
	}
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
