// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// Telling a machine fetching a link from a person opening it.
//
// confirm_token.opened_at is evidence. A grant's demonstrability rests partly
// on the ask-to-click chain a later reader follows from the row: the mail went
// out at issued_at, the person opened it at opened_at, the answer landed at
// consumed_at. That sentence has a named data subject as its subject.
//
// Every GET of the confirm page used to stamp it, and most GETs of a link in a
// mail are not people. Mail security products fetch every link before
// delivering the message; so do link expanders, preview generators, corporate
// proxies and the recipient's own mail client generating a thumbnail. Each of
// those wrote a line saying somebody opened their consent link, frequently at a
// moment they were asleep.
//
// AN INVENTED OPENING IS NOT A COSMETIC TELEMETRY ERROR. It is a false line in
// the record a controller would produce to show a consent was freely given, and
// it is false in the direction that flatters the controller — which is the
// direction an auditor reads hardest.
//
// WHAT THIS CANNOT DO, stated plainly because the limit is the interesting
// part. There is no way to prove a human is at the keyboard from one HTTP
// request, and this does not pretend to.
//
// It catches the scanners that ANNOUNCE themselves — every browser-initiated
// prefetch, prerender and preview, which is what mail clients and link
// expanders trigger. It catches NONE of the plain HTTP clients that send no
// Fetch Metadata at all, and a great many security scanners are exactly that.
// So this narrows the defect rather than closing it, and a later change that
// wants to close it needs a different signal than a request header.
//
// A REQUEST THAT SAYS NOTHING IS COUNTED AS A PERSON, which is the deliberate
// direction: over-recording a fetch as an opening is the defect being fixed,
// and a fix that swung far enough to suppress real openings would replace one
// untruth with another.
//
// ONE CASE FALLS THE OTHER WAY AND IS LEFT THERE. A browser that prerenders
// the page and then activates it for a subject who really does read it makes
// only the prerender request — the activation reuses what was fetched — so the
// opening goes unrecorded. That is under-recording, which leaves opened_at
// saying less than the truth rather than more, and a row that stays NULL
// claims nothing about anybody. Recording it would need a signal from the page
// after activation, which is a frontend change and its own decision.
//
// WHAT IT MUST NOT TEST is how the request was initiated. The confirm page is
// an SPA: the subject's browser navigates to the page, and the PAGE then calls
// this endpoint with fetch(), which carries Sec-Fetch-Mode: cors and
// Sec-Fetch-Dest: empty. Judging those would classify every genuine opening as
// a machine — the defect this exists to fix, inverted and applied to everybody.

import "net/http"

// FetchKind says what retrieved a link, as far as the request admits.
type FetchKind int

const (
	// FetchByAPerson is a request that presents as a human navigating to the
	// page: a top-level document navigation, or a request that says nothing
	// either way. The second half is the lenient direction — see the file
	// comment.
	FetchByAPerson FetchKind = iota
	// FetchByAMachine is a request that SAYS it is not a person opening the
	// page: a prefetch, a preview, a subresource load, a non-navigation.
	FetchByAMachine
)

// The Fetch Metadata headers a browser sends on its own, which a page cannot
// forge and which are exactly the ones a scanner's HTTP client either omits or
// fills in honestly.
const (
	headerSecPurpose = "Sec-Purpose"
	headerPurpose    = "Purpose"
	headerXPurpose   = "X-Purpose"
	headerXMoz       = "X-Moz"
)

// WhatFetchedThis reads a request and answers whether it may be recorded as a
// person opening the page.
//
// THE DECISION IS ONE-SIDED. Every arm below turns a person into a machine, and
// none turns a machine into a person: an empty request stays FetchByAPerson.
// The failure this exists to end is recording openings nobody made, so the
// question asked is "does this request DENY being a person opening the page",
// not "does it prove it is one" — which no request can.
func WhatFetchedThis(r *http.Request) FetchKind {
	// A PREFETCH SAYS SO. The Sec-Purpose header is set by the browser, not by
	// the page, precisely so a server can decline to treat a speculative load
	// as a visit. The two older spellings are Chrome's and Firefox's, still
	// sent by versions in the field.
	for _, header := range []string{headerSecPurpose, headerPurpose, headerXPurpose} {
		if r.Header.Get(header) != "" {
			return FetchByAMachine
		}
	}
	// Firefox's own link prefetch, which predates the standard header.
	if r.Header.Get(headerXMoz) != "" {
		return FetchByAMachine
	}
	// SEC-FETCH-MODE AND SEC-FETCH-DEST ARE DELIBERATELY NOT TESTED. See the
	// file comment: this endpoint is called by the confirm page's own fetch(),
	// so a genuine opening arrives as cors/empty and judging those would
	// suppress every one of them.
	return FetchByAPerson
}
